package repository

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"backend/internal/domain"
)

type statsTestRow struct {
	metric   string
	refID    sql.NullInt64
	label    sql.NullString
	intValue int64
	floatVal sql.NullFloat64
}

type statsTestRows struct {
	rows      []statsTestRow
	index     int
	scanErr   error
	iteration error
}

func (r *statsTestRows) Next() bool {
	return r.index < len(r.rows)
}

func (r *statsTestRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	row := r.rows[r.index]
	r.index++
	values := []any{row.metric, row.refID, row.label, row.intValue, row.floatVal}
	for i, value := range values {
		if err := assignStatsValue(dest[i], value); err != nil {
			return err
		}
	}
	return nil
}

func assignStatsValue(destination, value any) error {
	switch target := destination.(type) {
	case *string:
		v, ok := value.(string)
		if !ok {
			return errors.New("stats scan value is not a string")
		}
		*target = v
	case *sql.NullInt64:
		v, ok := value.(sql.NullInt64)
		if !ok {
			return errors.New("stats scan value is not a nullable int64")
		}
		*target = v
	case *sql.NullString:
		v, ok := value.(sql.NullString)
		if !ok {
			return errors.New("stats scan value is not a nullable string")
		}
		*target = v
	case *int64:
		v, ok := value.(int64)
		if !ok {
			return errors.New("stats scan value is not an int64")
		}
		*target = v
	case *sql.NullFloat64:
		v, ok := value.(sql.NullFloat64)
		if !ok {
			return errors.New("stats scan value is not a nullable float64")
		}
		*target = v
	default:
		return errors.New("unsupported stats scan destination")
	}
	return nil
}

func (r *statsTestRows) Err() error { return r.iteration }

func TestDecodeTeamStats(t *testing.T) {
	avgCloseTime := sql.NullFloat64{Float64: 42.5, Valid: true}
	rows := &statsTestRows{rows: []statsTestRow{
		{metric: "status", label: sql.NullString{String: "todo", Valid: true}, intValue: 2},
		{metric: "top_assignee", refID: sql.NullInt64{Int64: 7, Valid: true},
			label: sql.NullString{String: "Ari", Valid: true}, intValue: 3},
		{metric: "top_assignee", refID: sql.NullInt64{Int64: 8, Valid: true},
			label: sql.NullString{String: "Bea", Valid: true}, intValue: 3},
		{metric: "close_time", intValue: 4, floatVal: avgCloseTime},
		{metric: "comments", refID: sql.NullInt64{Int64: 21, Valid: true},
			label: sql.NullString{String: "First", Valid: true}, intValue: 2},
		{metric: "comments", refID: sql.NullInt64{Int64: 22, Valid: true},
			label: sql.NullString{String: "Second", Valid: true}, intValue: 5},
		{metric: "total_comments", intValue: 7},
	}}

	stats, err := decodeTeamStats(rows, 5)
	if err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	if stats.TeamID != 5 || stats.TasksByStatus[string(domain.StatusTodo)] != 2 {
		t.Errorf("unexpected status metrics: %+v", stats)
	}
	if len(stats.TopAssignees30d) != 2 || stats.TopAssignees30d[0].UserID != 7 {
		t.Errorf("assignees not ordered by task count then user id: %+v", stats.TopAssignees30d)
	}
	if stats.ClosedTasks != 4 || stats.AvgCloseTimeSeconds == nil || *stats.AvgCloseTimeSeconds != 42.5 {
		t.Errorf("unexpected close time metrics: %+v", stats)
	}
	if stats.TotalComments != 7 || len(stats.CommentsByTask) != 2 || stats.CommentsByTask[0].TaskID != 22 {
		t.Errorf("unexpected comment metrics: %+v", stats)
	}
}

func TestDecodeTeamStatsReturnsScanError(t *testing.T) {
	rows := &statsTestRows{rows: []statsTestRow{{metric: "status"}}}
	rows.scanErr = errors.New("scan failed")
	_, err := decodeTeamStats(rows, 5)
	if err == nil || !strings.Contains(err.Error(), "scan team stats") {
		t.Fatalf("got %v, want contextual scan error", err)
	}
}
