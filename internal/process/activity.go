package process

import "time"

// ActivitySample is one targeted observation of a PID-reuse-safe process.
// Cumulative counters are kept in memory only long enough to derive deltas.
type ActivitySample struct {
	Key   Key
	Taken time.Time

	CPUTime          time.Duration
	CPUKnown         bool
	DiskReadBytes    uint64
	DiskWrittenBytes uint64
	IOKnown          bool
	RSSBytes         uint64

	OpenFiles               int
	WritableRepositoryFiles int
	FilesKnown              bool

	Sockets           int
	ConnectedSockets  int
	ReceiveQueueBytes uint64
	SendQueueBytes    uint64
	SocketsKnown      bool

	Note string
}

// ActivityDelta is the safe, persistable difference between two samples.
// Known flags are independent: a platform may expose counters while denying
// descriptor inspection, and callers must not turn either denial into zero.
type ActivityDelta struct {
	Key        Key
	Taken      time.Time
	Interval   time.Duration
	BaselineOK bool

	CPUPercent       float64
	CPUDelta         time.Duration
	CPUKnown         bool
	DiskReadBytes    uint64
	DiskWrittenBytes uint64
	IOKnown          bool

	OpenFiles               int
	WritableRepositoryFiles int
	FilesKnown              bool

	Sockets           int
	ConnectedSockets  int
	ReceiveQueueBytes uint64
	SendQueueBytes    uint64
	NetworkChanged    bool
	SocketsKnown      bool

	Note string
}

// DeriveActivity returns a delta only when both samples name the same process,
// time advanced, and every cumulative counter moved monotonically.
func DeriveActivity(previous, current ActivitySample) ActivityDelta {
	d := ActivityDelta{
		Key:                     current.Key,
		Taken:                   current.Taken,
		OpenFiles:               current.OpenFiles,
		WritableRepositoryFiles: current.WritableRepositoryFiles,
		FilesKnown:              current.FilesKnown,
		Sockets:                 current.Sockets,
		ConnectedSockets:        current.ConnectedSockets,
		ReceiveQueueBytes:       current.ReceiveQueueBytes,
		SendQueueBytes:          current.SendQueueBytes,
		SocketsKnown:            current.SocketsKnown,
		Note:                    current.Note,
	}
	if previous.Key != current.Key || previous.Taken.IsZero() || !current.Taken.After(previous.Taken) {
		return d
	}
	d.Interval = current.Taken.Sub(previous.Taken)
	if previous.CPUKnown && current.CPUKnown && current.CPUTime >= previous.CPUTime {
		d.CPUDelta = current.CPUTime - previous.CPUTime
		d.CPUPercent = 100 * float64(d.CPUDelta) / float64(d.Interval)
		d.CPUKnown = true
		d.BaselineOK = true
	}
	if previous.IOKnown && current.IOKnown &&
		current.DiskReadBytes >= previous.DiskReadBytes &&
		current.DiskWrittenBytes >= previous.DiskWrittenBytes {
		d.DiskReadBytes = current.DiskReadBytes - previous.DiskReadBytes
		d.DiskWrittenBytes = current.DiskWrittenBytes - previous.DiskWrittenBytes
		d.IOKnown = true
		d.BaselineOK = true
	}
	if previous.SocketsKnown && current.SocketsKnown {
		d.NetworkChanged = previous.Sockets != current.Sockets ||
			previous.ConnectedSockets != current.ConnectedSockets ||
			previous.ReceiveQueueBytes != current.ReceiveQueueBytes ||
			previous.SendQueueBytes != current.SendQueueBytes
	}
	return d
}
