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
	GID           string         `json:"-"`
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
	ErrNotFound     = errors.New("download not found")
	ErrBadStatus    = errors.New("invalid status")
	allowedStatuses = map[DownloadStatus]bool{
		StatusActive:    true,
		StatusPaused:    true,
		StatusCancelled: true,
	}
)

func (d *Downloads) ToJSON(w io.Writer) error { return json.NewEncoder(w).Encode(d) }

func (d *Download) ToJSON(w io.Writer) error { return json.NewEncoder(w).Encode(d) }

func (d *Download) FromJSON(r io.Reader) error { return json.NewDecoder(r).Decode(d) }

func GetDownloads() Downloads {
	return downloadList
}

func AddDownload(d *Download) {
	d.ID = getNextID()
	d.DesiredStatus = StatusActive
	d.Status = StatusActive
	downloadList = append(downloadList, d)
}

func FindByID(id int) (*Download, error) {
	for _, dl := range downloadList {
		if dl.ID == id {
			return dl, nil
		}
	}
	return nil, ErrNotFound
}

func UpdateDesiredStatus(id int, newStatus DownloadStatus) (*Download, error) {
	if !allowedStatuses[newStatus] {
		return nil, ErrBadStatus
	}
	dl, err := FindByID(id)
	if err != nil {
		return nil, err
	}

	dl.DesiredStatus = newStatus
	dl.Status = newStatus

	return dl, nil
}

func ParseID(s string) (int, error) { return strconv.Atoi(s) }

func getNextID() int {
	dl := downloadList[len(downloadList)-1]
	return dl.ID + 1
}

var downloadList = []*Download{
	&Download{
		ID:         1,
		GID:        "1",
		Source:     "magnet:?xt=urn:btih:a216611be5b8d8c6306748d132774aa514977ee8&dn=Chappelle%27s+Show+%5B2003%5D&tr=http%3A%2F%2Ftracker.openbittorrent.com%3A80%2Fannounce&tr=udp%3A%2F%2Ftracker.leechers-paradise.org%3A6969%2Fannounce&tr=udp%3A%2F%2Fzer0day.to%3A1337%2Fannounce&tr=http%3A%2F%2Ftracker.opentrackr.org%3A1337%2Fannounce&tr=udp%3A%2F%2Ftracker.internetwarriors.net%3A1337%2Fannounce&tr=http%3A%2F%2Fexplodie.org%3A6969%2Fannounce&tr=http%3A%2F%2F5.79.83.193%3A2710%2Fannounce&tr=udp%3A%2F%2Ftracker.coppersurfer.tk%3A6969%2Fannounce&tr=http%3A%2F%2Fbt.henbt.com%3A2710%2Fannounce&tr=udp%3A%2F%2F9.rarbg.com%3A2710%2Fannounce",
		TargetPath: "/tv/",
		Status:     StatusQueued,
		CreatedAt:  time.Now(),
	},
	&Download{
		ID:         2,
		GID:        "2",
		Source:     "magnet:?xt=urn:btih:1300da4907fcec1470bb3cd31563bb401cd33c14&dn=Superman+%282025%29+En+2160p+UHD+X265+HEVC+10+bit+Dolby+Digital+Plus%5BMulti-Sub%5D.mkv&tr=udp%3A%2F%2Ftracker.opentrackr.org%3A1337%2Fannounce&tr=udp%3A%2F%2Fzephir.monocul.us%3A6969%2Fannounce&tr=udp%3A%2F%2Ftracker.leechers-paradise.org%3A6969%2Fannounce&tr=udp%3A%2F%2Ftracker.pomf.se%3A80%2Fannounce&tr=udp%3A%2F%2Ftracker.tiny-vps.com%3A6969%2Fannounce&tr=http%3A%2F%2Fipv4.rer.lol%3A2710%2Fannounce&tr=http%3A%2F%2Fhome.yxgz.club%3A6969%2Fannounce&tr=http%3A%2F%2Fbt.okmp3.ru%3A2710%2Fannounce&tr=http%3A%2F%2Fbt1.xxxxbt.cc%3A6969%2Fannounce&tr=http%3A%2F%2F207.241.226.111%3A6969%2Fannounce",
		TargetPath: "/movies/",
		Status:     StatusQueued,
		CreatedAt:  time.Now(),
	},
}
