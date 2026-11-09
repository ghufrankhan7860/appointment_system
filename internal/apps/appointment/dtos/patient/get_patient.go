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
	Relationship     string `json:"relationship" enums:"self,father,mother,spouse,child,sibling,guardian,caregiver,other" example:"self"`
	AccessLevel      string `json:"accessLevel" enums:"full,appointments_only,read_only" example:"full"`
}
