package appointment

type CancelAppointmentRequest struct {
	ID     uint64 `json:"-" uri:"id" swaggerignore:"true"`
	Reason string `json:"reason" example:"Feeling better"`
}
