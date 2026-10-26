package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	"backend/internal/domain"
)

// StatsRepo — интерфейс репозитория аналитики.
type StatsRepo interface {
	TeamStats(ctx context.Context, teamID int64) (domain.TeamStats, error)
}

type statsRepo struct {
	db DB
}

func NewStatsRepo(db DB) StatsRepo {
	return &statsRepo{db: db}
}

// teamStatsQuery собирает весь отчёт по команде одним запросом.
// Каждая метрика — отдельная CTE, результаты объединяются через UNION ALL
// в строки вида (metric, ref_id, label, int_value, float_value).
// Все CTE опираются на team_tasks, поэтому данные других команд в отчёт не попадают.
const teamStatsQuery = `
WITH team_tasks AS (
    SELECT id, title, status, assignee_id, created_at, closed_at
    FROM tasks
    WHERE team_id = ?
),
status_counts AS (
    SELECT status, COUNT(*) AS cnt
    FROM team_tasks
    GROUP BY status
),
top_assignees AS (
    SELECT u.id AS user_id, u.name, COUNT(*) AS closed_cnt
    FROM team_tasks t
    JOIN users u ON u.id = t.assignee_id
    WHERE t.status = 'done'
      AND t.closed_at >= UTC_TIMESTAMP(3) - INTERVAL 30 DAY
    GROUP BY u.id, u.name
    ORDER BY closed_cnt DESC, u.id
    LIMIT 3
),
close_time AS (
    SELECT COUNT(*) AS closed_cnt,
           AVG(TIMESTAMPDIFF(SECOND, created_at, closed_at)) AS avg_seconds
    FROM team_tasks
    WHERE status = 'done' AND closed_at IS NOT NULL
),
comment_counts AS (
    SELECT t.id AS task_id, t.title, COUNT(c.id) AS cnt
    FROM team_tasks t
    LEFT JOIN task_comments c ON c.task_id = t.id
    GROUP BY t.id, t.title
)
SELECT 'status' AS metric, CAST(NULL AS SIGNED) AS ref_id, status AS label,
       cnt AS int_value, CAST(NULL AS DOUBLE) AS float_value
FROM status_counts
UNION ALL
SELECT 'top_assignee', user_id, name, closed_cnt, NULL FROM top_assignees
UNION ALL
SELECT 'close_time', NULL, NULL, closed_cnt, avg_seconds FROM close_time
UNION ALL
SELECT 'comments', task_id, title, cnt, NULL FROM comment_counts
`

func (r *statsRepo) TeamStats(ctx context.Context, teamID int64) (domain.TeamStats, error) {
	stats := domain.TeamStats{
		TeamID: teamID,
		TasksByStatus: map[string]int64{
			string(domain.StatusTodo):       0,
			string(domain.StatusInProgress): 0,
			string(domain.StatusDone):       0,
		},
		TopAssignees30d: make([]domain.AssigneeStat, 0, 3),
		CommentsByTask:  make([]domain.TaskCommentStat, 0),
	}

	rows, err := r.db.Conn(ctx).QueryContext(ctx, teamStatsQuery, teamID)
	if err != nil {
		return stats, fmt.Errorf("select team stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			metric   string
			refID    sql.NullInt64
			label    sql.NullString
			intVal   int64
			floatVal sql.NullFloat64
		)
		if err = rows.Scan(&metric, &refID, &label, &intVal, &floatVal); err != nil {
			return stats, fmt.Errorf("scan team stats: %w", err)
		}

		switch metric {
		case "status":
			stats.TasksByStatus[label.String] = intVal
		case "top_assignee":
			stats.TopAssignees30d = append(stats.TopAssignees30d, domain.AssigneeStat{
				UserID: refID.Int64, Name: label.String, ClosedTasks: intVal,
			})
		case "close_time":
			stats.ClosedTasks = intVal
			if floatVal.Valid {
				stats.AvgCloseTimeSeconds = &floatVal.Float64
			}
		case "comments":
			stats.TotalComments += intVal
			stats.CommentsByTask = append(stats.CommentsByTask, domain.TaskCommentStat{
				TaskID: refID.Int64, Title: label.String, Comments: intVal,
			})
		}
	}
	if err = rows.Err(); err != nil {
		return stats, fmt.Errorf("iterate team stats: %w", err)
	}

	// UNION ALL не гарантирует порядок строк внутри частей — сортируем здесь.
	sort.SliceStable(stats.TopAssignees30d, func(i, j int) bool {
		a, b := stats.TopAssignees30d[i], stats.TopAssignees30d[j]
		if a.ClosedTasks != b.ClosedTasks {
			return a.ClosedTasks > b.ClosedTasks
		}
		return a.UserID < b.UserID
	})
	sort.Slice(stats.CommentsByTask, func(i, j int) bool {
		return stats.CommentsByTask[i].TaskID < stats.CommentsByTask[j].TaskID
	})

	return stats, nil
}
