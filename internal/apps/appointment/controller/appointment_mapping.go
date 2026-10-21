package controller

import (
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/dtos/appointment"
	"appointment_system/internal/apps/appointment/models"
	"time"
)

func toRescheduleProposalResponse(proposal *models.RescheduleProposal, location *time.Location) appointment.RescheduleProposalResponse {
	newStartsAt := proposal.NewStartsAt.In(location)

	return appointment.RescheduleProposalResponse{
		ID:                       proposal.ID,
		AppointmentID:            proposal.AppointmentID,
		PlaceholderAppointmentID: proposal.PlaceholderAppointmentID,
		Reason:                   string(proposal.Reason),
		Note:                     proposal.Note,
		Status:                   string(proposal.Status),
		ExpiresAt:                proposal.ExpiresAt.In(location),
		NewStartsAt:              newStartsAt,
		NewEndsAt:                proposal.NewEndsAt.In(location),
		NewDate:                  newStartsAt.Format(constant.DateLayout),
		NewStartTime:             newStartsAt.Format(constant.ClockLayout),
		NewMode:                  string(proposal.NewMode),
		CreatedAt:                proposal.CreatedAt.In(location),
	}
}

func toAppointmentResponse(details *models.AppointmentDetails, location *time.Location) appointment.AppointmentResponse {
	startsAt := details.StartsAt.In(location)

	response := appointment.AppointmentResponse{
		ID:              details.ID,
		StartsAt:        startsAt,
		EndsAt:          details.EndsAt.In(location),
		Date:            startsAt.Format(constant.DateLayout),
		StartTime:       startsAt.Format(constant.ClockLayout),
		Mode:            string(details.Mode),
		Status:          string(details.Status),
		ConsultationFee: details.ConsultationFee,
		SlotDurationMin: details.SlotDurationMin,
		Notes:           details.PatientNotes,
		CancelReason:    string(details.CancelReason),
		RescheduledFrom: details.RescheduledFromID,
		PaymentStatus:   string(details.PaymentStatus),
		Doctor: appointment.DoctorSummary{
			ID:         details.DoctorID,
			Name:       details.DoctorName,
			Speciality: details.DoctorSpeciality,
		},
		Patient: appointment.PatientSummary{
			ID:   details.PatientID,
			Name: details.PatientName,
		},
		BookedBy: appointment.BookedBySummary{
			UserID:              details.BookedByUserID,
			PatientRelationship: string(details.BookedByRelationship),
		},
	}
	if details.PaymentDueAt != nil {
		paymentDueAt := details.PaymentDueAt.In(location)
		response.PaymentDueAt = &paymentDueAt
	}
	if details.CancelledAt != nil {
		cancelledAt := details.CancelledAt.In(location)
		response.CancelledAt = &cancelledAt
	}

	return response
}
