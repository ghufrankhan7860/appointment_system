package doctor

type AvailabilityRuleRequest struct {
	DayOfWeek int    `json:"dayOfWeek" binding:"min=0,max=6" example:"1"`
	StartTime string `json:"startTime" binding:"required" example:"09:00"`
	EndTime   string `json:"endTime" binding:"required" example:"13:00"`
	Mode      string `json:"mode" binding:"required" enums:"in_person,video,phone" example:"in_person"`
}

type ReplaceAvailabilityRequest struct {
	Rules []AvailabilityRuleRequest `json:"rules" binding:"dive"`
}

type AvailabilityRuleResponse struct {
	DayOfWeek int    `json:"dayOfWeek" example:"1"`
	Day       string `json:"day" example:"Monday"`
	StartTime string `json:"startTime" example:"09:00"`
	EndTime   string `json:"endTime" example:"13:00"`
	Mode      string `json:"mode" example:"in_person"`
}

type ReplaceAvailabilityResponse struct {
	Rules []AvailabilityRuleResponse `json:"rules"`
}
