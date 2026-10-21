package appointment

type RescheduleAppointmentRequest struct {
	ID       uint64 `json:"-" uri:"id" swaggerignore:"true"`
	StartsAt string `json:"startsAt" binding:"required" example:"2026-10-27T10:00:00+05:30"`
	Mode     string `json:"mode" enums:"in_person,video,phone" example:"in_person"`

	IdempotencyKey string `json:"-" swaggerignore:"true"`
}
