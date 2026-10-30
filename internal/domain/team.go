package domain

import "time"

type Team struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedBy int64     `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	// Role is the current user's role in the team.
	Role Role `json:"role,omitempty"`
}

type TeamMember struct {
	TeamID int64 `json:"team_id"`
	UserID int64 `json:"user_id"`
	Role   Role  `json:"role"`
}
