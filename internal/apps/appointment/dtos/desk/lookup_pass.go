package desk

// LookupPassRequest is a scanned pass token or a typed pass code.
type LookupPassRequest struct {
	Code string `json:"code" binding:"required" example:"K7Q2-M9XD"`
}

// DeskPatientResponse is what the desk needs to book for a patient.
type DeskPatientResponse struct {
	ID          uint64 `json:"id"`
	Name        string `json:"name"`
	Gender      string `json:"gender" example:"Female"`
	DateOfBirth string `json:"dateOfBirth" example:"1990-01-02"`
	BloodType   string `json:"bloodType" example:"O+"`
	Contact     string `json:"contact,omitempty" example:"9876543210"`
}
