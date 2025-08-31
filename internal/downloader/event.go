package downloader

// Event represents a state change or progress update from a downloader.
//
// Type indicates what kind of event occurred. For terminal events
// (e.g. Complete, Failed, Cancelled) the reconciler will update the
// repository's Status and clear the GID where appropriate. Progress
// events carry transient information about the download and do not
// mutate repository state yet.
type Event struct {
    ID       int
    GID      string
    Type     EventType
    Progress *Progress
    Meta     *Meta
}

// EventType defines the set of events that downloaders may emit.
type EventType string

const (
    EventStart     EventType = "Start"
    EventPaused    EventType = "Paused"
    EventCancelled EventType = "Cancelled"
    EventComplete  EventType = "Complete"
    EventFailed    EventType = "Failed"
    EventProgress  EventType = "Progress"
    EventMeta      EventType = "Meta"
)

// Progress provides optional details about an in-progress download.
// Values are left generic so downloaders can supply whatever metrics
// they have available (e.g. bytes downloaded, total size).
type Progress struct {
    Completed int64
    Total     int64
    // Speed is the current download speed in bytes/sec, if available.
    // A value of 0 indicates it was not provided by the adapter.
    Speed     int64
}

// Meta carries optional metadata about a download that should be persisted
// by the reconciler, such as the resolved resource name.
type Meta struct {
    Name *string
}
