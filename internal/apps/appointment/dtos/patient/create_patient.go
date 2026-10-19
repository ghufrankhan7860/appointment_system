package patient

type CreatePatientRequest struct {
	Name             string `json:"name" binding:"required"`
	Gender           string `json:"gender" binding:"required"`
	DateOfBirth      string `json:"dateOfBirth" binding:"required"`
	BloodType        string `json:"bloodType" binding:"required"`
	EmergencyContact string `json:"emergencyContact"`
	Relationship     string `json:"relationship" binding:"required"`
}

type CreatePatientResponse struct {
	ID               uint64 `json:"id"`
	Name             string `json:"name"`
	Gender           string `json:"gender"`
	DateOfBirth      string `json:"dateOfBirth"`
	BloodType        string `json:"bloodType"`
	EmergencyContact string `json:"emergencyContact"`
}
