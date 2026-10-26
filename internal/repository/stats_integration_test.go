package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/domain"
)

// Интеграционный тест SQL-отчёта. Требует MySQL: переменные DB_* как у приложения.
// Внутри docker compose запускается командой `make test`.
func TestTeamStatsIntegration(t *testing.T) {
	if os.Getenv("DB_HOST") == "" {
		t.Skip("DB_HOST is not set, skipping integration test")
	}

	migrations := os.Getenv("MIGRATIONS_PATH")
	if migrations == "" {
		migrations = "file://../database/migrations"
	}

	db, err := database.New(config.GetDSN(
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_DATABASE"),
	), migrations)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	q := db.Conn(ctx)
	suffix := time.Now().UnixNano()

	exec := func(query string, args ...any) int64 {
		t.Helper()
		res, err := q.ExecContext(ctx, query, args...)
		if err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
		id, _ := res.LastInsertId()
		return id
	}

	newUser := func(name string) int64 {
		return exec(`INSERT INTO users (email, password_hash, name) VALUES (?, 'x', ?)`,
			fmt.Sprintf("%s-%d@test.local", name, suffix), name)
	}

	u1, u2, u3, u4 := newUser("u1"), newUser("u2"), newUser("u3"), newUser("u4")

	teamA := exec(`INSERT INTO teams (name, created_by) VALUES ('A', ?)`, u1)
	teamB := exec(`INSERT INTO teams (name, created_by) VALUES ('B', ?)`, u1)

	// newTask создаёт задачу, созданную createdDaysAgo дней назад и закрытую closedDaysAgo дней назад
	// (closedDaysAgo < 0 — задача не закрыта).
	newTask := func(team int64, status domain.TaskStatus, assignee *int64, createdDaysAgo, closedDaysAgo int) int64 {
		var closed any
		if closedDaysAgo >= 0 {
			closed = closedDaysAgo
		}
		return exec(`
			INSERT INTO tasks (team_id, title, description, status, created_by, assignee_id, created_at, closed_at)
			VALUES (?, 'task', '', ?, ?, ?,
			        UTC_TIMESTAMP(3) - INTERVAL ? DAY,
			        IF(? IS NULL, NULL, UTC_TIMESTAMP(3) - INTERVAL ? DAY))`,
			team, status, u1, assignee, createdDaysAgo, closed, closed)
	}

	comment := func(task int64, n int) {
		for i := 0; i < n; i++ {
			exec(`INSERT INTO task_comments (task_id, user_id, content) VALUES (?, ?, 'c')`, task, u1)
		}
	}

	// Команда A.
	t1 := newTask(teamA, domain.StatusDone, &u2, 10, 8) // 2 дня
	newTask(teamA, domain.StatusDone, &u2, 5, 4)        // 1 день
	newTask(teamA, domain.StatusDone, &u3, 3, 1)        // 2 дня
	newTask(teamA, domain.StatusDone, &u4, 60, 40)      // 20 дней, вне окна 30 дней
	newTask(teamA, domain.StatusDone, &u1, 2, 1)        // 1 день
	t5 := newTask(teamA, domain.StatusTodo, &u3, 1, -1) // открыта
	newTask(teamA, domain.StatusInProgress, nil, 1, -1) // без исполнителя
	comment(t1, 2)
	comment(t5, 1)

	// Команда B — её данные не должны попасть в отчёт команды A.
	tb := newTask(teamB, domain.StatusDone, &u4, 2, 1)
	newTask(teamB, domain.StatusDone, &u4, 3, 1)
	newTask(teamB, domain.StatusDone, &u4, 4, 1)
	comment(tb, 5)

	stats, err := NewStatsRepo(db).TeamStats(ctx, teamA)
	if err != nil {
		t.Fatalf("team stats: %v", err)
	}

	wantStatus := map[string]int64{"todo": 1, "in_progress": 1, "done": 5}
	for status, want := range wantStatus {
		if got := stats.TasksByStatus[status]; got != want {
			t.Errorf("tasks_by_status[%s] = %d, want %d", status, got, want)
		}
	}

	// u2 — 2 закрытые, u1 и u3 — по одной (при равенстве сортировка по id).
	// u4 закрыл задачу команды A 40 дней назад, а задачи команды B не учитываются.
	wantTop := []domain.AssigneeStat{
		{UserID: u2, Name: "u2", ClosedTasks: 2},
		{UserID: u1, Name: "u1", ClosedTasks: 1},
		{UserID: u3, Name: "u3", ClosedTasks: 1},
	}
	if len(stats.TopAssignees30d) != len(wantTop) {
		t.Fatalf("top assignees = %+v, want %+v", stats.TopAssignees30d, wantTop)
	}
	for i, want := range wantTop {
		if stats.TopAssignees30d[i] != want {
			t.Errorf("top[%d] = %+v, want %+v", i, stats.TopAssignees30d[i], want)
		}
	}

	if stats.ClosedTasks != 5 {
		t.Errorf("closed_tasks = %d, want 5", stats.ClosedTasks)
	}

	// (2 + 1 + 2 + 20 + 1) дней / 5 задач.
	const day = 86400.0
	wantAvg := (2 + 1 + 2 + 20 + 1) * day / 5
	if stats.AvgCloseTimeSeconds == nil {
		t.Fatal("avg_close_time_seconds is nil")
	}
	if diff := *stats.AvgCloseTimeSeconds - wantAvg; diff > 1 || diff < -1 {
		t.Errorf("avg_close_time_seconds = %f, want %f", *stats.AvgCloseTimeSeconds, wantAvg)
	}

	if stats.TotalComments != 3 {
		t.Errorf("total_comments = %d, want 3", stats.TotalComments)
	}
	if len(stats.CommentsByTask) != 7 {
		t.Errorf("comments_by_task has %d tasks, want 7", len(stats.CommentsByTask))
	}
	for _, c := range stats.CommentsByTask {
		if c.TaskID == tb {
			t.Errorf("task %d of another team leaked into stats", tb)
		}
	}

	// Пустая команда — нулевые метрики без ошибок.
	empty, err := NewStatsRepo(db).TeamStats(ctx, exec(`INSERT INTO teams (name, created_by) VALUES ('C', ?)`, u1))
	if err != nil {
		t.Fatalf("empty team stats: %v", err)
	}
	if empty.ClosedTasks != 0 || empty.AvgCloseTimeSeconds != nil || len(empty.TopAssignees30d) != 0 || empty.TotalComments != 0 {
		t.Errorf("unexpected stats for empty team: %+v", empty)
	}
}
