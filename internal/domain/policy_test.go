package domain

import "testing"

func ptr[T any](v T) *T { return &v }

func TestCanInvite(t *testing.T) {
	tests := []struct {
		actor, target Role
		want          bool
	}{
		{RoleOwner, RoleMember, true},
		{RoleOwner, RoleAdmin, true},
		{RoleOwner, RoleOwner, false},
		{RoleAdmin, RoleMember, true},
		{RoleAdmin, RoleAdmin, false},
		{RoleAdmin, RoleOwner, false},
		{RoleMember, RoleMember, false},
		{RoleMember, RoleAdmin, false},
		{"", RoleMember, false},
	}

	for _, tt := range tests {
		if got := CanInvite(tt.actor, tt.target); got != tt.want {
			t.Errorf("CanInvite(%q, %q) = %v, want %v", tt.actor, tt.target, got, tt.want)
		}
	}
}

func TestCanChangeRole(t *testing.T) {
	tests := []struct {
		name                 string
		actor, current, newR Role
		want                 bool
	}{
		{"owner promotes member", RoleOwner, RoleMember, RoleAdmin, true},
		{"owner demotes admin", RoleOwner, RoleAdmin, RoleMember, true},
		{"owner cannot grant owner", RoleOwner, RoleMember, RoleOwner, false},
		{"owner cannot change owner", RoleOwner, RoleOwner, RoleMember, false},
		{"admin cannot change roles", RoleAdmin, RoleMember, RoleAdmin, false},
		{"admin cannot demote owner", RoleAdmin, RoleOwner, RoleMember, false},
		{"member cannot change roles", RoleMember, RoleMember, RoleAdmin, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanChangeRole(tt.actor, tt.current, tt.newR); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCanViewStats(t *testing.T) {
	if !CanViewStats(RoleOwner) || !CanViewStats(RoleAdmin) {
		t.Error("owner and admin must see stats")
	}
	if CanViewStats(RoleMember) || CanViewStats("") {
		t.Error("member and outsider must not see stats")
	}
}

func TestCanEditTask(t *testing.T) {
	const (
		creator  int64 = 1
		assignee int64 = 2
		other    int64 = 3
	)

	task := Task{ID: 10, TeamID: 1, CreatedBy: creator, AssigneeID: ptr(assignee)}

	statusOnly := TaskPatch{Status: ptr(StatusDone)}
	titleChange := TaskPatch{Title: ptr("new")}
	reassign := TaskPatch{AssigneeID: ptr(other)}
	full := TaskPatch{Title: ptr("t"), Description: ptr("d"), Status: ptr(StatusInProgress), AssigneeID: ptr(other)}

	tests := []struct {
		name   string
		userID int64
		role   Role
		patch  TaskPatch
		want   bool
	}{
		{"owner edits anything", other, RoleOwner, full, true},
		{"admin edits anything", other, RoleAdmin, full, true},
		{"creator edits all fields", creator, RoleMember, full, true},
		{"assignee changes status", assignee, RoleMember, statusOnly, true},
		{"assignee cannot change title", assignee, RoleMember, titleChange, false},
		{"assignee cannot reassign", assignee, RoleMember, reassign, false},
		{"other member cannot edit", other, RoleMember, statusOnly, false},
		{"non-member cannot edit", creator, "", statusOnly, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanEditTask(tt.userID, tt.role, task, tt.patch); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCanEditTaskUnassigned(t *testing.T) {
	task := Task{CreatedBy: 1}
	if CanEditTask(2, RoleMember, task, TaskPatch{Status: ptr(StatusDone)}) {
		t.Error("member must not edit unassigned task of someone else")
	}
}
