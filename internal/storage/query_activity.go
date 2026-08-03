package storage

import (
	"context"
	"fmt"
	"strings"
)

// ActivityFilter narrows activity history. Results are newest first.
type ActivityFilter struct {
	ProcUID   string
	SessionID string
	SinceNs   int64
	Limit     int
}

const activityColumns = `id, proc_uid, session_id, ts_ns, interval_ns, baseline_ok,
	cpu_percent, cpu_delta_ns, cpu_known,
	disk_read_bytes, disk_written_bytes, io_known, rss_bytes,
	open_files, writable_repository_files, files_known,
	sockets, connected_sockets, receive_queue_bytes, send_queue_bytes,
	network_changed, sockets_known, note`

// ListActivity returns bounded activity evidence, newest first.
func (s *Store) ListActivity(ctx context.Context, f ActivityFilter) ([]ActivityRecord, error) {
	q := `SELECT ` + activityColumns + ` FROM process_activity`
	var where []string
	var args []any
	if f.ProcUID != "" {
		where = append(where, "proc_uid = ?")
		args = append(args, f.ProcUID)
	}
	if f.SessionID != "" {
		where = append(where, "session_id = ?")
		args = append(args, f.SessionID)
	}
	if f.SinceNs > 0 {
		where = append(where, "ts_ns >= ?")
		args = append(args, f.SinceNs)
	}
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY ts_ns DESC, id DESC LIMIT ?"
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: listing activity: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ActivityRecord
	for rows.Next() {
		var rec ActivityRecord
		if err := rows.Scan(
			&rec.ID, &rec.ProcUID, &rec.SessionID, &rec.TsNs, &rec.IntervalNs, &rec.BaselineOK,
			&rec.CPUPercent, &rec.CPUDeltaNs, &rec.CPUKnown,
			&rec.DiskReadBytes, &rec.DiskWrittenBytes, &rec.IOKnown, &rec.RSSBytes,
			&rec.OpenFiles, &rec.WritableRepositoryFiles, &rec.FilesKnown,
			&rec.Sockets, &rec.ConnectedSockets, &rec.ReceiveQueueBytes, &rec.SendQueueBytes,
			&rec.NetworkChanged, &rec.SocketsKnown, &rec.Note,
		); err != nil {
			return nil, fmt.Errorf("storage: scanning activity: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}
