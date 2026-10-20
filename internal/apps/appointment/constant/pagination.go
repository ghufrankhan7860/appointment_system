package constant

// List endpoints page with these when the client sends no limit, and never
// return more than MaxListLimit items at once.
const (
	DefaultListLimit = 20
	MaxListLimit     = 100
)
