package storage

import "fmt"

// InsertActivity appends one bounded, derived activity sample.
func (t *Tx) InsertActivity(rec ActivityRecord) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT INTO process_activity (
			proc_uid, session_id, ts_ns, interval_ns, baseline_ok,
			cpu_percent, cpu_delta_ns, cpu_known,
			disk_read_bytes, disk_written_bytes, io_known, rss_bytes,
			open_files, writable_repository_files, files_known,
			sockets, connected_sockets, receive_queue_bytes, send_queue_bytes,
			network_changed, sockets_known, note
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ProcUID, rec.SessionID, rec.TsNs, rec.IntervalNs, rec.BaselineOK,
		rec.CPUPercent, rec.CPUDeltaNs, rec.CPUKnown,
		rec.DiskReadBytes, rec.DiskWrittenBytes, rec.IOKnown, rec.RSSBytes,
		rec.OpenFiles, rec.WritableRepositoryFiles, rec.FilesKnown,
		rec.Sockets, rec.ConnectedSockets, rec.ReceiveQueueBytes, rec.SendQueueBytes,
		rec.NetworkChanged, rec.SocketsKnown, rec.Note)
	if err != nil {
		return fmt.Errorf("storage: inserting activity for %s: %w", rec.ProcUID, err)
	}
	return nil
}
