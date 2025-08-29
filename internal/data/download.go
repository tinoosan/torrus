package data

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"time"
)

type Download struct {
	ID            int            `json:"id"`
	Source        string         `json:"source"`
	TargetPath    string         `json:"targetPath"`
	Status        DownloadStatus `json:"status"`
	DesiredStatus DownloadStatus `json:"desiredStatus,omitempty"`
	CreatedAt     time.Time      `json:"createdAt"`
}

const (
	StatusQueued    DownloadStatus = "Queued"
	StatusActive    DownloadStatus = "Active"
	StatusPaused    DownloadStatus = "Paused"
	StatusComplete  DownloadStatus = "Complete"
	StatusCancelled DownloadStatus = "Cancelled"
	StatusError     DownloadStatus = "Failed"
)

type Downloads []*Download
type DownloadStatus string

var (
	ErrNotFound      = errors.New("download not found")
	ErrBadStatus     = errors.New("invalid status")
	ErrInvalidSource = errors.New("invalid source")
	ErrTargetPath    = errors.New("invalid target path")
	)

func (d *Downloads) ToJSON(w io.Writer) error { return json.NewEncoder(w).Encode(d) }

func (d *Download) ToJSON(w io.Writer) error { return json.NewEncoder(w).Encode(d) }

func (d *Download) FromJSON(r io.Reader) error { return json.NewDecoder(r).Decode(d) }

func (d *Download) Clone() *Download {
	if d == nil {
		return nil
	}
	cp := *d
	return &cp
}

func (ds Downloads) Clone() Downloads {
	out := make(Downloads, len(ds))
	for i, d := range ds {
		if d != nil {
			out[i] = d.Clone()
		}
	}
	return out
}
func ParseID(s string) (int, error) { return strconv.Atoi(s) }
