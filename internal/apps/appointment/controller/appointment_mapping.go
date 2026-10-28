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

func toDoctorPatientResponse(patient *models.DoctorPatient, location *time.Location, now time.Time) appointment.DoctorPatientResponse {
	response := appointment.DoctorPatientResponse{
		ID:     patient.PatientID,
		Name:   patient.Name,
		Age:    ageOn(patient.DateOfBirth, now.In(location)),
		Gender: string(patient.Gender),
		Visits: patient.Visits,
	}
	if patient.LastAppointmentAt != nil {
		lastAppointmentAt := patient.LastAppointmentAt.In(location)
		response.LastAppointmentAt = &lastAppointmentAt
	}
	if patient.NextAppointmentAt != nil {
		nextAppointmentAt := patient.NextAppointmentAt.In(location)
		response.NextAppointmentAt = &nextAppointmentAt
	}

	return response
}

// ageOn returns the whole years from a date of birth to today's clinic date.
func ageOn(dateOfBirth time.Time, today time.Time) int {
	age := today.Year() - dateOfBirth.Year()
	if today.Month() < dateOfBirth.Month() || (today.Month() == dateOfBirth.Month() && today.Day() < dateOfBirth.Day()) {
		age--
	}

	return age
}
