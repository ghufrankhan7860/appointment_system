package constant

import "time"

// Rules for a doctor adding clinic staff through an invite code. Codes use
// PatientInviteCodeLength and PatientInviteCodeAlphabet.
const (
	// StaffInviteTTL is how long a staff invite stays redeemable.
	StaffInviteTTL = 48 * time.Hour
	// MaxPendingStaffInvites caps a doctor's open staff invites.
	MaxPendingStaffInvites = 5
	// MaxStaffInviteCodeTries bounds new codes drawn when one clashes.
	MaxStaffInviteCodeTries = 3
	// MaxFailedStaffInviteAttempts wrong codes per client IP within
	// StaffInviteAttemptWindow block further sign-ups until the window passes.
	MaxFailedStaffInviteAttempts = 10
	StaffInviteAttemptWindow     = time.Hour
)
