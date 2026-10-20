package doctor

import "time"

type ListSlotsRequest struct {
	ID   uint64 `uri:"id"`
	Date string `form:"date"`
}

type SlotResponse struct {
	StartsAt  time.Time `json:"startsAt" example:"2026-10-26T09:00:00+05:30"`
	EndsAt    time.Time `json:"endsAt" example:"2026-10-26T09:15:00+05:30"`
	StartTime string    `json:"startTime" example:"09:00"`
	Mode      string    `json:"mode" example:"in_person"`
}

type ListSlotsResponse struct {
	Date     string         `json:"date" example:"2026-10-26"`
	Timezone string         `json:"timezone" example:"Asia/Kolkata"`
	Slots    []SlotResponse `json:"slots"`
}
