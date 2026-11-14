package desk

// CreateWalkInRequest adds a patient who has no app. Contact is a phone
// number for the clinic to reach them.
type CreateWalkInRequest struct {
	Name        string `json:"name" binding:"required" example:"Asha Verma"`
	Gender      string `json:"gender" binding:"required" enums:"Male,Female,Other,Unknown" example:"Female"`
	DateOfBirth string `json:"dateOfBirth" binding:"required" example:"1990-01-02"`
	BloodType   string `json:"bloodType" binding:"required" enums:"A+,A-,B+,B-,AB+,AB-,O+,O-" example:"O+"`
	Contact     string `json:"contact" binding:"max=30" example:"9876543210"`
}
