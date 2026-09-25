package constants

const (
	RoleSurveyor   = "surveyor"
	RoleGISAnalyst = "gis_analyst"
	RoleReviewer   = "reviewer"
	RoleAuditor    = "auditor"
	RoleAdmin      = "admin"
)

func ValidRole(role string) bool {
	return role == RoleSurveyor || role == RoleGISAnalyst || role == RoleReviewer || role == RoleAuditor || role == RoleAdmin
}
