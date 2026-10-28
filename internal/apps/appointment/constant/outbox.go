package constant

import "time"

// Rules for handing outbox events to a publisher.
const (
	// OutboxSendTimeout is how long one event's publish may take. Publishing
	// runs while the batch's rows are locked, so a stuck send must not hold
	// them for long.
	OutboxSendTimeout = 10 * time.Second
)
