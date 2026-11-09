package constant

import "time"

// Rules for sharing a patient with another user through an invite code.
const (
	// PatientInviteTTL is how long an invite stays redeemable when its creator
	// does not choose.
	PatientInviteTTL = 48 * time.Hour
	// PatientInviteMinTTL and PatientInviteMaxTTL bound a chosen lifetime.
	PatientInviteMinTTL = time.Hour
	PatientInviteMaxTTL = 7 * 24 * time.Hour
	// MaxPendingPatientInvites caps the open invites on one patient.
	MaxPendingPatientInvites = 5
	// PatientInviteCodeLength and PatientInviteCodeAlphabet shape a code: upper
	// case without the look-alikes I, O, 0 and 1, so it can be read out loud.
	PatientInviteCodeLength   = 8
	PatientInviteCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	// MaxPatientInviteCodeTries bounds new codes drawn when one clashes with an
	// existing code.
	MaxPatientInviteCodeTries = 3
	// MaxFailedPatientInviteAttempts wrong codes per user within
	// PatientInviteAttemptWindow block further redeems until the window passes.
	MaxFailedPatientInviteAttempts = 5
	PatientInviteAttemptWindow     = time.Hour
)
