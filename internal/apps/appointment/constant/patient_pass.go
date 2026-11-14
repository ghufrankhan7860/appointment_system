package constant

import "time"

// Rules for the pass a patient shows at a clinic desk: a QR token and a short
// code typed at a desk without a camera. The code uses PatientInviteCodeLength
// and PatientInviteCodeAlphabet.
const (
	// PatientPassTTL is how long a pass works; the patient's screen renews it.
	PatientPassTTL = 10 * time.Minute
	// PatientPassTokenBytes is the random size of the QR token, kept short so
	// the QR stays small and easy to scan.
	PatientPassTokenBytes = 16
	// MaxPatientPassCodeTries bounds new codes drawn when one clashes.
	MaxPatientPassCodeTries = 3
	// MaxFailedPatientPassAttempts wrong codes per desk user within
	// PatientPassAttemptWindow block further lookups until the window passes.
	MaxFailedPatientPassAttempts = 10
	PatientPassAttemptWindow     = time.Hour
)
