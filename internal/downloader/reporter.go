package downloader

// Reporter publishes downloader events.
type Reporter interface {
	Report(Event)
}

// ChanReporter writes events to a channel.
type ChanReporter struct {
	ch chan<- Event
}

func NewChanReporter(ch chan<- Event) *ChanReporter { return &ChanReporter{ch: ch} }

func (r *ChanReporter) Report(e Event) {
	if r == nil {
		return
	}
	r.ch <- e
}
