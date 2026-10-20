package constant

// Limits for doctor availability and slots.
const (
	// MaxAvailabilityRules is the most weekly blocks one doctor can have.
	MaxAvailabilityRules = 50
	// SlotHorizonDays is how far ahead slots can be listed.
	SlotHorizonDays = 90
	// OverrideHorizonDays is how far ahead a leave or extra session can be added.
	OverrideHorizonDays = 365
	// MaxOverrideReasonLength is the longest reason text on an override.
	MaxOverrideReasonLength = 500
)
