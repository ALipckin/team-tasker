package domain

type AssigneeStat struct {
	UserID      int64  `json:"user_id"`
	Name        string `json:"name"`
	ClosedTasks int64  `json:"closed_tasks"`
}

type TaskCommentStat struct {
	TaskID   int64  `json:"task_id"`
	Title    string `json:"title"`
	Comments int64  `json:"comments"`
}

type TeamStats struct {
	TeamID              int64            `json:"team_id"`
	TasksByStatus       map[string]int64 `json:"tasks_by_status"`
	TopAssignees30d     []AssigneeStat   `json:"top_assignees_30d"`
	ClosedTasks         int64            `json:"closed_tasks"`
	AvgCloseTimeSeconds *float64         `json:"avg_close_time_seconds"`
	TotalComments       int64            `json:"total_comments"`
	// CommentsByTask contains up to 10 tasks with the most comments.
	CommentsByTask []TaskCommentStat `json:"comments_by_task"`
}
