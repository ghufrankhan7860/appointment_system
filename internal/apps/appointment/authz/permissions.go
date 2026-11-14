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
	PermPaymentVerify       Permission = "payment:verify"
	PermRecordWrite         Permission = "record:write"
	PermRecordAudit         Permission = "record:audit"
	PermPatientView         Permission = "patient:view"
	PermSessionManage       Permission = "session:manage"
	PermCallHost            Permission = "call:host"
	// PermAppointmentDesk is the clinic desk: the doctor or their staff booking
	// for a patient and running the doctor's day (list, cancel, move visits).
	PermAppointmentDesk Permission = "appointment:desk"
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
		PermSessionManage,
	},
	models.RoleDoctor: {
		PermDoctorProfileManage,
		PermAppointmentManage,
		PermAppointmentDesk,
		PermRecordWrite,
		PermPatientView,
		PermCallHost,
		PermPaymentVerify,
	},
	models.RolePatient: {
		PermAppointmentBook,
		PermPaymentPay,
	},
	// Staff run one doctor's desk; never records, calls or the doctor's profile.
	models.RoleStaff: {
		PermAppointmentDesk,
		PermPaymentVerify,
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
