package services

import "appointment_system/internal/apps/appointment/models"

func findLink(links []*models.UserPatient, userID uint64) *models.UserPatient {
	for _, link := range links {
		if link.UserID == userID {
			return link
		}
	}

	return nil
}

// fullHoldersAfter counts the full holders left once userID's link has
// accessLevel, or is gone when accessLevel is empty.
func fullHoldersAfter(links []*models.UserPatient, userID uint64, accessLevel models.AccessLevel) int {
	count := 0
	for _, link := range links {
		level := link.AccessLevel
		if link.UserID == userID {
			level = accessLevel
		}
		if level == models.AccessLevelFull {
			count++
		}
	}

	return count
}

func hasSelfLink(links []*models.UserPatient) bool {
	for _, link := range links {
		if link.PatientRelationship == models.RelationshipSelf {
			return true
		}
	}

	return false
}

func patientAccessEvent(
	eventType string,
	link *models.UserPatient,
	actingUserID uint64,
	reason models.UserPatientChangeReason,
) *models.OutboxEvent {
	return &models.OutboxEvent{
		AggregateType: models.OutboxAggregatePatient,
		AggregateID:   link.PatientID,
		EventType:     eventType,
		Payload: map[string]any{
			"patientId":       link.PatientID,
			"userId":          link.UserID,
			"changedByUserId": actingUserID,
			"relationship":    link.PatientRelationship,
			"accessLevel":     link.AccessLevel,
			"status":          link.Status,
			"reason":          reason,
		},
	}
}
