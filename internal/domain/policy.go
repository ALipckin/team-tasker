package domain

// TaskPatch — набор полей, которые пользователь хочет изменить в задаче.
// nil означает «поле не меняется».
type TaskPatch struct {
	Title       *string
	Description *string
	Status      *TaskStatus
	// AssigneeID: nil — не менять, указатель на 0 — снять исполнителя.
	AssigneeID *int64
}

func (p TaskPatch) Empty() bool {
	return p.Title == nil && p.Description == nil && p.Status == nil && p.AssigneeID == nil
}

// CanInvite — может ли пользователь с ролью actor пригласить участника с ролью target.
// Роль создателя приглашением не выдаётся; назначать администраторов может только создатель.
func CanInvite(actor, target Role) bool {
	switch target {
	case RoleOwner:
		return false
	case RoleMember:
		return actor.IsManager()
	case RoleAdmin:
		return actor == RoleOwner
	default:
		return false
	}
}

// CanChangeRole — может ли actor сменить роль участника с currentRole на newRole.
// Менять роли может только создатель; роль создателя не меняется и не выдаётся.
func CanChangeRole(actor, currentRole, newRole Role) bool {
	if actor != RoleOwner {
		return false
	}

	if currentRole == RoleOwner || newRole == RoleOwner {
		return false
	}

	return newRole == RoleAdmin || newRole == RoleMember
}

// CanViewStats — аналитика доступна только создателю и администратору.
func CanViewStats(role Role) bool {
	return role.IsManager()
}

// CanEditTask проверяет, может ли пользователь userID с ролью role применить patch к task.
// Проверку того, что новый исполнитель состоит в команде, делает сервис.
func CanEditTask(userID int64, role Role, task Task, patch TaskPatch) bool {
	if role.IsManager() {
		return true
	}

	if role != RoleMember {
		return false
	}

	if task.CreatedBy == userID {
		return true
	}

	if task.AssigneeID != nil && *task.AssigneeID == userID {
		// Исполнитель может менять только статус.
		return patch.Title == nil && patch.Description == nil && patch.AssigneeID == nil
	}

	return false
}
