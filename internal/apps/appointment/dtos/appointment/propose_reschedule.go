package appointment

type ProposeRescheduleRequest struct {
	ID       uint64 `json:"-" uri:"id" swaggerignore:"true"`
	StartsAt string `json:"startsAt" binding:"required" example:"2026-10-27T11:00:00+05:30"`
	Mode     string `json:"mode" enums:"in_person,video,phone" example:"in_person"`
	Reason   string `json:"reason" binding:"required" enums:"emergency,personal,clinic_closed,other" example:"emergency"`
	Note     string `json:"note" example:"Called in for an emergency surgery"`
}
