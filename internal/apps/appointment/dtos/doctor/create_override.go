package doctor

// CreateOverrideRequest without times and not available is a whole day off;
// with times it is a partial leave, or an extra session when available.
type CreateOverrideRequest struct {
	Date        string `json:"date" binding:"required" example:"2026-10-26"`
	StartTime   string `json:"startTime" example:"14:00"`
	EndTime     string `json:"endTime" example:"17:00"`
	IsAvailable bool   `json:"isAvailable" example:"false"`
	Mode        string `json:"mode" enums:"in_person,video,phone" example:"in_person"`
	Reason      string `json:"reason" example:"Conference"`
}

type OverrideResponse struct {
	ID          uint64 `json:"id"`
	Date        string `json:"date" example:"2026-10-26"`
	StartTime   string `json:"startTime,omitempty" example:"14:00"`
	EndTime     string `json:"endTime,omitempty" example:"17:00"`
	IsAvailable bool   `json:"isAvailable"`
	Mode        string `json:"mode"`
	Reason      string `json:"reason"`
}
