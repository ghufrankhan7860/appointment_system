package admin

type ReviewDoctorRequest struct {
	ID     uint64 `json:"-" uri:"id" swaggerignore:"true"`
	Reason string `json:"reason" example:"Registration number not found in the council register"`
}
