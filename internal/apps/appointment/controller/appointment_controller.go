// Package controller implements appointment application actions and use cases.
//
// Create one focused method for each action, such as creating, cancelling, or listing
// appointments. Accept validated DTOs, perform application-level authorization, convert
// inputs to domain values, call services, and shape output DTOs. Keep HTTP framework and
// persistence types outside this package.
package controller

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/dtos/appointment"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"context"
	"time"
)

type AppointmentController struct {
	appointmentService *services.AppointmentService
}

func NewAppointmentController(appointmentService *services.AppointmentService) (*AppointmentController, error) {
	if appointmentService == nil {
		return nil, apperrors.ErrNilAppointmentService
	}
	return &AppointmentController{
		appointmentService: appointmentService,
	}, nil
}

func (ac *AppointmentController) Book(ctx context.Context, request *appointment.BookAppointmentRequest) (*appointment.AppointmentResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	startsAt, err := time.Parse(constant.InstantLayout, request.StartsAt)
	if err != nil {
		return nil, apperrors.Invalid(apperrors.ErrInvalidAppointmentInput, "startsAt", "must be a time with its UTC offset, like 2026-10-26T09:00:00+05:30")
	}

	newAppointment := models.Appointment{
		PatientID:      request.PatientID,
		DoctorID:       request.DoctorID,
		StartsAt:       startsAt,
		Mode:           models.ConsultationMode(request.Mode),
		PatientNotes:   request.Notes,
		IdempotencyKey: request.IdempotencyKey,
	}
	bookedAppointment, err := ac.appointmentService.Book(ctx, userID, &newAppointment)
	if err != nil {
		return nil, err
	}

	response := toAppointmentResponse(bookedAppointment, ac.appointmentService.Location())
	return &response, nil
}

func (ac *AppointmentController) GetByID(ctx context.Context, request *appointment.GetAppointmentRequest) (*appointment.AppointmentResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	foundAppointment, err := ac.appointmentService.GetByID(ctx, userID, role, request.ID)
	if err != nil {
		return nil, err
	}

	response := toAppointmentResponse(foundAppointment, ac.appointmentService.Location())
	return &response, nil
}

func (ac *AppointmentController) List(ctx context.Context, request *appointment.ListAppointmentsRequest) (*appointment.ListAppointmentsResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	filter := models.AppointmentFilter{
		PatientID: request.PatientID,
		Status:    models.AppointmentStatus(request.Status),
		Limit:     request.Limit,
		Offset:    request.Offset,
	}
	if request.From != "" {
		if filter.From, err = time.Parse(constant.InstantLayout, request.From); err != nil {
			return nil, apperrors.Invalid(apperrors.ErrInvalidAppointmentInput, "from", "must be a time with its UTC offset, like 2026-10-26T00:00:00+05:30")
		}
	}
	if request.To != "" {
		if filter.To, err = time.Parse(constant.InstantLayout, request.To); err != nil {
			return nil, apperrors.Invalid(apperrors.ErrInvalidAppointmentInput, "to", "must be a time with its UTC offset, like 2026-10-27T00:00:00+05:30")
		}
	}

	appointments, err := ac.appointmentService.List(ctx, userID, filter)
	if err != nil {
		return nil, err
	}

	location := ac.appointmentService.Location()
	response := appointment.ListAppointmentsResponse{
		Appointments: make([]appointment.AppointmentResponse, 0, len(appointments)),
	}
	for _, listedAppointment := range appointments {
		response.Appointments = append(response.Appointments, toAppointmentResponse(listedAppointment, location))
	}

	return &response, nil
}

func (ac *AppointmentController) Cancel(ctx context.Context, request *appointment.CancelAppointmentRequest) (*appointment.AppointmentResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	cancelledAppointment, err := ac.appointmentService.Cancel(ctx, userID, role, request.ID, request.Reason)
	if err != nil {
		return nil, err
	}

	response := toAppointmentResponse(cancelledAppointment, ac.appointmentService.Location())
	return &response, nil
}

func (ac *AppointmentController) Reschedule(ctx context.Context, request *appointment.RescheduleAppointmentRequest) (*appointment.AppointmentResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	startsAt, err := time.Parse(constant.InstantLayout, request.StartsAt)
	if err != nil {
		return nil, apperrors.Invalid(apperrors.ErrInvalidAppointmentInput, "startsAt", "must be a time with its UTC offset, like 2026-10-26T09:00:00+05:30")
	}

	rescheduledAppointment, err := ac.appointmentService.Reschedule(ctx, userID, role, request.ID, startsAt, models.ConsultationMode(request.Mode), request.IdempotencyKey)
	if err != nil {
		return nil, err
	}

	response := toAppointmentResponse(rescheduledAppointment, ac.appointmentService.Location())
	return &response, nil
}

func (ac *AppointmentController) Complete(ctx context.Context, request *appointment.CloseAppointmentRequest) (*appointment.AppointmentResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	completedAppointment, err := ac.appointmentService.Complete(ctx, userID, role, request.ID)
	if err != nil {
		return nil, err
	}

	response := toAppointmentResponse(completedAppointment, ac.appointmentService.Location())
	return &response, nil
}

func (ac *AppointmentController) MarkNoShow(ctx context.Context, request *appointment.CloseAppointmentRequest) (*appointment.AppointmentResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	missedAppointment, err := ac.appointmentService.MarkNoShow(ctx, userID, role, request.ID)
	if err != nil {
		return nil, err
	}

	response := toAppointmentResponse(missedAppointment, ac.appointmentService.Location())
	return &response, nil
}

func (ac *AppointmentController) ProposeReschedule(ctx context.Context, request *appointment.ProposeRescheduleRequest) (*appointment.RescheduleProposalResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	startsAt, err := time.Parse(constant.InstantLayout, request.StartsAt)
	if err != nil {
		return nil, apperrors.Invalid(apperrors.ErrInvalidAppointmentInput, "startsAt", "must be a time with its UTC offset, like 2026-10-26T09:00:00+05:30")
	}

	proposal, err := ac.appointmentService.ProposeReschedule(ctx, userID, role, request.ID, startsAt, models.ConsultationMode(request.Mode), models.ProposalReason(request.Reason), request.Note)
	if err != nil {
		return nil, err
	}

	response := toRescheduleProposalResponse(proposal, ac.appointmentService.Location())
	return &response, nil
}

func (ac *AppointmentController) GetRescheduleProposal(ctx context.Context, request *appointment.GetRescheduleProposalRequest) (*appointment.RescheduleProposalResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	proposal, err := ac.appointmentService.GetPendingProposal(ctx, userID, role, request.ID)
	if err != nil {
		return nil, err
	}

	response := toRescheduleProposalResponse(proposal, ac.appointmentService.Location())
	return &response, nil
}

func (ac *AppointmentController) AcceptReschedule(ctx context.Context, request *appointment.AnswerRescheduleProposalRequest) (*appointment.AppointmentResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	movedAppointment, err := ac.appointmentService.AcceptReschedule(ctx, userID, role, request.ID)
	if err != nil {
		return nil, err
	}

	response := toAppointmentResponse(movedAppointment, ac.appointmentService.Location())
	return &response, nil
}

func (ac *AppointmentController) DeclineReschedule(ctx context.Context, request *appointment.AnswerRescheduleProposalRequest) (*appointment.AppointmentResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	keptAppointment, err := ac.appointmentService.DeclineReschedule(ctx, userID, role, request.ID)
	if err != nil {
		return nil, err
	}

	response := toAppointmentResponse(keptAppointment, ac.appointmentService.Location())
	return &response, nil
}

func (ac *AppointmentController) ListPatients(ctx context.Context, request *appointment.ListDoctorPatientsRequest) (*appointment.ListDoctorPatientsResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	patients, err := ac.appointmentService.ListPatients(ctx, userID, role, request.Query, request.Limit, request.Offset)
	if err != nil {
		return nil, err
	}

	location := ac.appointmentService.Location()
	now := time.Now()
	response := appointment.ListDoctorPatientsResponse{
		Patients: make([]appointment.DoctorPatientResponse, 0, len(patients)),
	}
	for _, listedPatient := range patients {
		response.Patients = append(response.Patients, toDoctorPatientResponse(listedPatient, location, now))
	}

	return &response, nil
}

func (ac *AppointmentController) ListForDoctor(ctx context.Context, request *appointment.ListDoctorAppointmentsRequest) (*appointment.ListDoctorAppointmentsResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	location := ac.appointmentService.Location()
	date := time.Now().In(location)
	if request.Date != "" {
		if date, err = time.Parse(constant.DateLayout, request.Date); err != nil {
			return nil, apperrors.Invalid(apperrors.ErrInvalidAppointmentInput, "date", "must be YYYY-MM-DD")
		}
	}

	appointments, err := ac.appointmentService.ListForDoctor(ctx, userID, role, date)
	if err != nil {
		return nil, err
	}

	response := appointment.ListDoctorAppointmentsResponse{
		Date:         date.Format(constant.DateLayout),
		Timezone:     location.String(),
		Appointments: make([]appointment.AppointmentResponse, 0, len(appointments)),
	}
	for _, listedAppointment := range appointments {
		response.Appointments = append(response.Appointments, toAppointmentResponse(listedAppointment, location))
	}

	return &response, nil
}
