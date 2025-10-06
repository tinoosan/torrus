package repo

import (
    "context"
    "database/sql"
    "encoding/json"
    "errors"
    "net"
    "net/url"
    "os"
    "strings"
    "time"

    _ "github.com/jackc/pgx/v5/stdlib"

    "github.com/google/uuid"
    "github.com/tinoosan/torrus/internal/data"
    "github.com/tinoosan/torrus/internal/fp"
)

// PostgresRepo implements ExtendedRepo backed by PostgreSQL.
// It expects a table `downloads` with a unique index on `fingerprint`.
type PostgresRepo struct {
    db *sql.DB
}

// NewPostgresRepo constructs a repository using the provided DSN.
func NewPostgresRepo(dsn string) (*PostgresRepo, error) {
    db, err := sql.Open("pgx", dsn)
    if err != nil {
        return nil, err
    }
    // Verify connection
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := db.PingContext(ctx); err != nil {
        _ = db.Close()
        return nil, err
    }
    r := &PostgresRepo{db: db}
    if err := r.ensureSchema(ctx); err != nil {
        _ = db.Close()
        return nil, err
    }
    return r, nil
}

// NewPostgresRepoFromEnv constructs a DSN using component env vars.
// Recognized envs (with defaults):
//   POSTGRES_HOST (postgres), POSTGRES_PORT (5432), POSTGRES_DB (torrus),
//   POSTGRES_USER (torrus), POSTGRES_PASSWORD (empty), POSTGRES_SSLMODE (disable)
// Credentials and db name are URL-encoded to handle special characters safely.
func NewPostgresRepoFromEnv() (*PostgresRepo, error) {
    host := getenv("POSTGRES_HOST", "postgres")
    port := getenv("POSTGRES_PORT", "5432")
    db := getenv("POSTGRES_DB", "torrus")
    user := getenv("POSTGRES_USER", "torrus")
    pass := getenv("POSTGRES_PASSWORD", "")
    ssl := getenv("POSTGRES_SSLMODE", "disable")

    u := &url.URL{
        Scheme: "postgres",
        User:   url.UserPassword(user, pass),
        Host:   net.JoinHostPort(host, port),
        Path:   "/" + db,
    }
    q := url.Values{}
    q.Set("sslmode", ssl)
    u.RawQuery = q.Encode()
    return NewPostgresRepo(u.String())
}

func getenv(k, def string) string {
    if v := os.Getenv(k); v != "" {
        return v
    }
    return def
}

func (r *PostgresRepo) Close() error { return r.db.Close() }

func (r *PostgresRepo) ensureSchema(ctx context.Context) error {
    // Create table if not exists
    _, err := r.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS downloads (
    id UUID PRIMARY KEY,
    gid TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL,
    target_path TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    files JSONB,
    status TEXT NOT NULL,
    desired_status TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    fingerprint TEXT NOT NULL UNIQUE
);
`)
    return err
}

// List implements DownloadReader.List
func (r *PostgresRepo) List(ctx context.Context) (data.Downloads, error) {
    rows, err := r.db.QueryContext(ctx, `SELECT id,gid,source,target_path,name,files,status,desired_status,created_at FROM downloads ORDER BY created_at ASC`)
    if err != nil { return nil, err }
    defer rows.Close()
    var out data.Downloads
    for rows.Next() {
        dl, err := scanDownload(rows)
        if err != nil { return nil, err }
        out = append(out, dl)
    }
    return out, rows.Err()
}

// Get implements DownloadReader.Get
func (r *PostgresRepo) Get(ctx context.Context, id string) (*data.Download, error) {
    row := r.db.QueryRowContext(ctx, `SELECT id,gid,source,target_path,name,files,status,desired_status,created_at FROM downloads WHERE id=$1`, id)
    dl, err := scanDownload(row)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) { return nil, data.ErrNotFound }
        return nil, err
    }
    return dl, nil
}

// Add implements DownloadWriter.Add (no fingerprint enforcement)
func (r *PostgresRepo) Add(ctx context.Context, d *data.Download) (*data.Download, error) {
    id := uuid.NewString()
    filesJSON, _ := json.Marshal(d.Files)
    _, err := r.db.ExecContext(ctx, `INSERT INTO downloads (id,gid,source,target_path,name,files,status,desired_status,created_at,fingerprint) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
        id, d.GID, d.Source, d.TargetPath, d.Name, nullJSON(filesJSON), string(d.Status), string(d.DesiredStatus), d.CreatedAt, fp.Fingerprint(d.Source, d.TargetPath))
    if err != nil { return nil, err }
    return r.Get(ctx, id)
}

// AddWithFingerprint implements atomic check-then-insert based on fingerprint.
func (r *PostgresRepo) AddWithFingerprint(ctx context.Context, d *data.Download, fprint string) (*data.Download, bool, error) {
    id := uuid.NewString()
    filesJSON, _ := json.Marshal(d.Files)
    // Try insert; on conflict do nothing, then fetch existing
    err := r.db.QueryRowContext(ctx, `
WITH ins AS (
    INSERT INTO downloads (id,gid,source,target_path,name,files,status,desired_status,created_at,fingerprint)
    VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
    ON CONFLICT (fingerprint) DO NOTHING
    RETURNING id
)
SELECT id FROM ins
`, id, d.GID, d.Source, d.TargetPath, d.Name, nullJSON(filesJSON), string(d.Status), string(d.DesiredStatus), d.CreatedAt, fprint).Scan(&id)
    if err != nil && !errors.Is(err, sql.ErrNoRows) {
        return nil, false, err
    }
    if err == nil {
        // Inserted new row
        dl, err := r.Get(ctx, id)
        return dl, true, err
    }
    // Fetch existing by fingerprint
    dl, err := r.GetByFingerprint(ctx, fprint)
    if err != nil { return nil, false, err }
    return dl, false, nil
}

// Update implements DownloadWriter.Update by fetching, mutating, and writing back with conflict detection.
func (r *PostgresRepo) Update(ctx context.Context, id string, mutate func(*data.Download) error) (*data.Download, error) {
    // Serialize updates per row using a transaction with SELECT ... FOR UPDATE
    tx, err := r.db.BeginTx(ctx, &sql.TxOptions{})
    if err != nil { return nil, err }
    defer func() {
        // Safe rollback when not committed
        _ = tx.Rollback()
    }()

    // Load the latest row under lock
    row := tx.QueryRowContext(ctx, `SELECT id,gid,source,target_path,name,files,status,desired_status,created_at FROM downloads WHERE id=$1 FOR UPDATE`, id)
    cur, err := scanDownload(row)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) { return nil, data.ErrNotFound }
        return nil, err
    }

    next := cur.Clone()
    if mutate != nil {
        if err := mutate(next); err != nil { return nil, err }
    }

    // If no effective change, return current
    if equalDownloads(cur, next) {
        if err := tx.Commit(); err != nil { return nil, err }
        return cur, nil
    }

    // Preserve original creation time (immutable) and write back other columns.
    // Recompute fingerprint for potential conflict
    newFP := fp.Fingerprint(next.Source, next.TargetPath)
    filesJSON, _ := json.Marshal(next.Files)

    if _, err := tx.ExecContext(ctx, `UPDATE downloads SET gid=$1, source=$2, target_path=$3, name=$4, files=$5, status=$6, desired_status=$7, fingerprint=$8 WHERE id=$9`,
        next.GID, next.Source, next.TargetPath, next.Name, nullJSON(filesJSON), string(next.Status), string(next.DesiredStatus), newFP, id); err != nil {
        if isUniqueViolation(err) {
            return nil, data.ErrConflict
        }
        return nil, err
    }

    // Return the updated snapshot from within the txn for consistency
    row2 := tx.QueryRowContext(ctx, `SELECT id,gid,source,target_path,name,files,status,desired_status,created_at FROM downloads WHERE id=$1`, id)
    updated, err := scanDownload(row2)
    if err != nil { return nil, err }
    if err := tx.Commit(); err != nil { return nil, err }
    return updated, nil
}

// Delete implements DownloadWriter.Delete
func (r *PostgresRepo) Delete(ctx context.Context, id string) error {
    res, err := r.db.ExecContext(ctx, `DELETE FROM downloads WHERE id=$1`, id)
    if err != nil { return err }
    n, _ := res.RowsAffected()
    if n == 0 { return data.ErrNotFound }
    return nil
}

// GetByFingerprint implements DownloadFinder
func (r *PostgresRepo) GetByFingerprint(ctx context.Context, fprint string) (*data.Download, error) {
    row := r.db.QueryRowContext(ctx, `SELECT id,gid,source,target_path,name,files,status,desired_status,created_at FROM downloads WHERE fingerprint=$1`, fprint)
    dl, err := scanDownload(row)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) { return nil, data.ErrNotFound }
        return nil, err
    }
    return dl, nil
}

// Helpers

type rowScanner interface{ Scan(dest ...any) error }

func scanDownload(rs rowScanner) (*data.Download, error) {
    var (
        id, gid, source, target, name, status, desired string
        created time.Time
        filesRaw sql.NullString
    )
    if err := rs.Scan(&id, &gid, &source, &target, &name, &filesRaw, &status, &desired, &created); err != nil {
        return nil, err
    }
    dl := &data.Download{
        ID:           id,
        GID:          gid,
        Source:       source,
        TargetPath:   target,
        Name:         name,
        Status:       data.DownloadStatus(status),
        DesiredStatus:data.DownloadStatus(desired),
        CreatedAt:    created,
    }
    if filesRaw.Valid && filesRaw.String != "" {
        _ = json.Unmarshal([]byte(filesRaw.String), &dl.Files)
    }
    return dl, nil
}

func equalDownloads(a, b *data.Download) bool {
    if a == nil || b == nil { return a == b }
    if a.ID != b.ID || a.GID != b.GID || a.Source != b.Source || a.TargetPath != b.TargetPath || a.Name != b.Name || a.Status != b.Status || a.DesiredStatus != b.DesiredStatus || !a.CreatedAt.Equal(b.CreatedAt) { return false }
    // compare files shallowly via JSON to avoid manual deep compare
    aj, _ := json.Marshal(a.Files)
    bj, _ := json.Marshal(b.Files)
    return string(aj) == string(bj)
}

func isUniqueViolation(err error) bool {
    // pgx stdlib returns error strings containing "duplicate key value violates unique constraint"
    if err == nil { return false }
    msg := strings.ToLower(err.Error())
    return strings.Contains(msg, "duplicate key value") || strings.Contains(msg, "unique constraint")
}

func nullJSON(b []byte) any {
    if len(b) == 0 || string(b) == "null" { return nil }
    return string(b)
}
