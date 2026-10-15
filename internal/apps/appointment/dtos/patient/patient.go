package patient

// DateLayout is the wire format for dateOfBirth.
const DateLayout = "2006-01-02"

type CreateRequest struct {
	Name             string `json:"name" binding:"required"`
	Gender           string `json:"gender" binding:"required"`
	DateOfBirth      string `json:"dateOfBirth" binding:"required"`
	BloodType        string `json:"bloodType"`
	EmergencyContact string `json:"emergencyContact"`
	Relationship     string `json:"relationship" binding:"required"`
}

type UpdateRequest struct {
	Name             string `json:"name" binding:"required"`
	Gender           string `json:"gender" binding:"required"`
	DateOfBirth      string `json:"dateOfBirth" binding:"required"`
	BloodType        string `json:"bloodType"`
	EmergencyContact string `json:"emergencyContact"`
}

type Response struct {
	ID               uint64 `json:"id"`
	Name             string `json:"name"`
	Gender           string `json:"gender"`
	DateOfBirth      string `json:"dateOfBirth"`
	BloodType        string `json:"bloodType"`
	EmergencyContact string `json:"emergencyContact"`
}
