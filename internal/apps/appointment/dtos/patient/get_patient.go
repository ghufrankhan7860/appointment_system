package patient

type GetPatientRequest struct {
	ID uint64 `uri:"id" binding:"required"`
}

type GetPatientResponse struct {
	ID               uint64 `json:"id"`
	Name             string `json:"name"`
	Gender           string `json:"gender"`
	DateOfBirth      string `json:"dateOfBirth"`
	BloodType        string `json:"bloodType"`
	EmergencyContact string `json:"emergencyContact"`
}
