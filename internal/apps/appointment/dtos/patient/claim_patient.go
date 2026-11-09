package patient

type ClaimPatientRequest struct {
	Code string `json:"code" binding:"required,max=20" example:"K7Q2-M9XD"`
}
