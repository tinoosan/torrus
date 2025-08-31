package data

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"time"
)

// Download represents a single file transfer managed by Torrus.
// It tracks the source URI, destination path and current state.
type Download struct {
    ID            int            `json:"id"`
    GID           string         `json:"gid"`
    Source        string         `json:"source"`
    TargetPath    string         `json:"targetPath"`
    // Name is a read-only field populated by the downloader via events.
    Name          string         `json:"name,omitempty"`
    // Files is an optional, read-only list of files for this download.
    // It is populated by downloader adapters when available.
    Files         []DownloadFile `json:"files,omitempty"`
    Status        DownloadStatus `json:"status"`
    DesiredStatus DownloadStatus `json:"desiredStatus,omitempty"`
    CreatedAt     time.Time      `json:"createdAt"`
}

// DownloadFile represents a single file within a multi-file download.
// All fields are optional, depending on downloader capabilities.
type DownloadFile struct {
    // Path is a relative path or filename for the file within the download.
    Path      string `json:"path"`
    // Length is the size of the file in bytes, if known.
    Length    int64  `json:"length,omitempty"`
    // Completed is the number of bytes downloaded for this file, if known.
    Completed int64  `json:"completed,omitempty"`
}

// Possible DownloadStatus values.
const (
    StatusQueued    DownloadStatus = "Queued"
    StatusActive    DownloadStatus = "Active"
    StatusResume    DownloadStatus = "Resume"
    StatusPaused    DownloadStatus = "Paused"
    StatusComplete  DownloadStatus = "Complete"
    StatusCancelled DownloadStatus = "Cancelled"
    StatusError     DownloadStatus = "Failed"
)

// Downloads is a slice of Download pointers.
type Downloads []*Download

// DownloadStatus represents the state of a Download.
type DownloadStatus string

var (
	// ErrNotFound indicates the requested download does not exist.
	ErrNotFound = errors.New("download not found")
	// ErrBadStatus indicates a provided status value is invalid.
	ErrBadStatus = errors.New("invalid status")
	// ErrInvalidSource is returned when a download source is empty or malformed.
	ErrInvalidSource = errors.New("invalid source")
	// ErrTargetPath signals that the provided target path is invalid.
	ErrTargetPath = errors.New("invalid target path")
)

// ToJSON writes the slice of downloads as JSON to the writer.
func (d *Downloads) ToJSON(w io.Writer) error { return json.NewEncoder(w).Encode(d) }

// ToJSON writes the download as JSON to the writer.
func (d *Download) ToJSON(w io.Writer) error { return json.NewEncoder(w).Encode(d) }

// FromJSON populates the download from JSON read from the reader.
func (d *Download) FromJSON(r io.Reader) error { return json.NewDecoder(r).Decode(d) }

// Clone returns a copy of the download. The receiver is left unchanged.
func (d *Download) Clone() *Download {
    if d == nil {
        return nil
    }
    cp := *d
    // Deep copy Files slice to avoid data races through shared backing arrays.
    if len(d.Files) > 0 {
        cp.Files = make([]DownloadFile, len(d.Files))
        copy(cp.Files, d.Files)
    }
    return &cp
}

// Clone returns copies of each download in the slice.
func (ds Downloads) Clone() Downloads {
	out := make(Downloads, len(ds))
	for i, d := range ds {
		if d != nil {
			out[i] = d.Clone()
		}
	}
	return out
}

// ParseID converts an ID string to an integer.
func ParseID(s string) (int, error) { return strconv.Atoi(s) }
