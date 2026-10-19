package patient

type CreatePatientRequest struct {
	Name             string `json:"name" binding:"required" example:"Asha Verma"`
	Gender           string `json:"gender" binding:"required" enums:"Male,Female,Other,Unknown" example:"Female"`
	DateOfBirth      string `json:"dateOfBirth" binding:"required" example:"1990-01-02"`
	BloodType        string `json:"bloodType" binding:"required" enums:"A+,A-,B+,B-,AB+,AB-,O+,O-" example:"O+"`
	EmergencyContact string `json:"emergencyContact" example:"9876543210"`
	Relationship     string `json:"relationship" binding:"required" enums:"self,father,mother,spouse,child,sibling,guardian,caregiver,other" example:"self"`
}

type CreatePatientResponse struct {
	ID               uint64 `json:"id"`
	Name             string `json:"name"`
	Gender           string `json:"gender"`
	DateOfBirth      string `json:"dateOfBirth" example:"1990-01-02"`
	BloodType        string `json:"bloodType"`
	EmergencyContact string `json:"emergencyContact"`
}
