package fp

import (
    "crypto/sha256"
    "encoding/hex"
    "path/filepath"
    "strings"
)

// NormalizeSource trims surrounding whitespace. Further normalization rules
// (e.g., URL normalization) can be added later as needed.
func NormalizeSource(s string) string {
    return strings.TrimSpace(s)
}

// NormalizeTargetPath trims whitespace and cleans the path using filepath.Clean.
// Note: On Unix (case-sensitive), we do not lowercase paths. If Windows support
// is added later, case normalization can be applied conditionally.
func NormalizeTargetPath(p string) string {
    p = strings.TrimSpace(p)
    if p == "" {
        return p
    }
    return filepath.Clean(p)
}

// Fingerprint computes a stable hex-encoded SHA-256 over the normalized
// source and targetPath. This is used to deduplicate identical requests.
func Fingerprint(source, targetPath string) string {
    ns := NormalizeSource(source)
    nt := NormalizeTargetPath(targetPath)
    h := sha256.New()
    // Use a separator that cannot be confused; NUL works for all inputs here.
    h.Write([]byte(ns))
    h.Write([]byte{0})
    h.Write([]byte(nt))
    sum := h.Sum(nil)
    return hex.EncodeToString(sum)
}

