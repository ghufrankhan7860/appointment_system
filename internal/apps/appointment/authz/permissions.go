// Package authz maps roles to named permissions. Every admin capability is a
// permission, so adding a narrower role (for example a reviewer) is a new map
// entry and never a handler change.
package authz

import "appointment_system/internal/apps/appointment/models"

type Permission string

const (
	PermAdminAccess         Permission = "admin:access"
	PermAdminManage         Permission = "admin:manage"
	PermDoctorVerify        Permission = "doctor:verify"
	PermDoctorSuspend       Permission = "doctor:suspend"
	PermSpecialityManage    Permission = "speciality:manage"
	PermDoctorProfileManage Permission = "doctor:profile"
	PermAppointmentBook     Permission = "appointment:book"
	PermAppointmentManage   Permission = "appointment:manage"
	PermPaymentPay          Permission = "payment:pay"
	PermPaymentRefund       Permission = "payment:refund"
	PermRecordWrite         Permission = "record:write"
	PermRecordAudit         Permission = "record:audit"
)

var rolePermissions = map[models.Role][]Permission{
	models.RoleAdmin: {
		PermAdminAccess,
		PermAdminManage,
		PermDoctorVerify,
		PermDoctorSuspend,
		PermSpecialityManage,
		PermPaymentRefund,
		PermRecordAudit,
	},
	models.RoleDoctor: {
		PermDoctorProfileManage,
		PermAppointmentManage,
		PermRecordWrite,
	},
	models.RolePatient: {
		PermAppointmentBook,
		PermPaymentPay,
	},
}

func HasPermission(role models.Role, permission Permission) bool {
	for _, granted := range rolePermissions[role] {
		if granted == permission {
			return true
		}
	}

	return false
}
