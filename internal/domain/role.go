package domain

type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

func (r Role) Valid() bool {
	return r == RoleOwner || r == RoleAdmin || r == RoleMember
}

// IsManager reports whether the role can manage a team.
func (r Role) IsManager() bool {
	return r == RoleOwner || r == RoleAdmin
}
