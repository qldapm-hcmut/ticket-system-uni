package postgres

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/db"
	platform "github.com/qldapm-hcmut/ticket-system-uni/internal/platform/postgres"
	a "github.com/qldapm-hcmut/ticket-system-uni/internal/report/application"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/shared/kernel"
)

type Store struct {
	runner *platform.TransactionRunner
	clock  kernel.Clock
}

func New(pool *pgxpool.Pool, clock kernel.Clock) *Store {
	return &Store{platform.NewTransactionRunner(pool), clock}
}

// Each report uses one snapshot, including authorization. Export bytes are not
// released to the caller until the audit transaction commits successfully.
func (s *Store) snapshot(ctx context.Context, actor, event string, fn func(*db.Queries, db.GetReportEventRow, pgtype.UUID) error) error {
	var aid, eid pgtype.UUID
	if aid.Scan(actor) != nil || eid.Scan(event) != nil || !aid.Valid || !eid.Valid {
		return a.ErrNotFound
	}
	return s.runner.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ"); err != nil {
			return err
		}
		q := db.New(tx)
		e, err := q.GetReportEvent(ctx, db.GetReportEventParams{ActorID: aid, EventID: eid})
		if errors.Is(err, pgx.ErrNoRows) {
			return a.ErrNotFound
		}
		if err != nil {
			return err
		}
		return fn(q, e, aid)
	})
}
func (s *Store) Summary(ctx context.Context, actor, event string) (out a.Summary, err error) {
	err = s.snapshot(ctx, actor, event, func(q *db.Queries, e db.GetReportEventRow, _ pgtype.UUID) error {
		r, err := q.GetReportSummary(ctx, e.ID)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		if !now.Before(e.StartAt.Time) {
			r.Waitlisted = 0
		}
		out = a.Summary{Capacity: e.Capacity, Registered: r.Registered, Waitlisted: r.Waitlisted, Cancelled: r.Cancelled, Promoted: r.Promoted, CheckedIn: r.CheckedIn, ByMethod: map[string]int64{"QR": r.Qr, "CODE": r.Code, "MANUAL": r.Manual}, GeneratedAt: now}
		if r.Registered > 0 {
			out.AttendanceRate = float64(r.CheckedIn) / float64(r.Registered)
		}
		return nil
	})
	return
}

// Prevent spreadsheet formula execution even for values prefixed by whitespace.
func safeCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if strings.HasPrefix(value, "\t") || strings.HasPrefix(value, "\r") || strings.HasPrefix(value, "\n") || (len(trimmed) > 0 && strings.ContainsRune("=+-@", rune(trimmed[0]))) {
		return "'" + value
	}
	return value
}
func renderCSV(rows []db.ListReportRegistrationsRow) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("\xef\xbb\xbf")
	w := csv.NewWriter(&b)
	w.UseCRLF = true
	if err := w.Write([]string{"STT", "MSSV", "Họ tên", "Email", "Trạng thái đăng ký", "Thời gian check-in", "Phương thức"}); err != nil {
		return nil, err
	}
	for i, r := range rows {
		at, method := "", ""
		if r.CheckedInAt.Valid {
			at = r.CheckedInAt.Time.Format(time.RFC3339)
		}
		if r.CheckinMethod.Valid {
			method = string(r.CheckinMethod.CheckinMethod)
		}
		record := []string{strconv.Itoa(i + 1), r.StudentCode.String, r.FullName, r.Email, string(r.Status), at, method}
		for j := range record {
			record[j] = safeCell(record[j])
		}
		if err := w.Write(record); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return b.Bytes(), w.Error()
}
func (s *Store) Export(ctx context.Context, actor, event, kind string) (out []byte, err error) {
	err = s.snapshot(ctx, actor, event, func(q *db.Queries, e db.GetReportEventRow, aid pgtype.UUID) error {
		rows, err := q.ListReportRegistrations(ctx, db.ListReportRegistrationsParams{EventID: e.ID, AttendanceOnly: kind == "attendance"})
		if err != nil {
			return err
		}
		out, err = renderCSV(rows)
		if err != nil {
			return err
		}
		metadata, err := json.Marshal(map[string]any{"kind": kind, "format": "csv", "rowCount": len(rows)})
		if err != nil {
			return err
		}
		return q.AuditReportExport(ctx, db.AuditReportExportParams{ActorID: aid, EventID: e.ID, Metadata: metadata})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
