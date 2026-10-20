package doctor

type GetAvailabilityResponse struct {
	Timezone  string                     `json:"timezone" example:"Asia/Kolkata"`
	Rules     []AvailabilityRuleResponse `json:"rules"`
	Overrides []OverrideResponse         `json:"overrides"`
}
