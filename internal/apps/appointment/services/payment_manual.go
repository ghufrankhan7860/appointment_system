package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/utils"
)

// The payment methods that move no money through the gateway, enabled by
// PAYMENT_METHODS: a direct UPI payment to the doctor, which the doctor (or an
// admin) checks by hand, and a fee collected at the clinic, which the doctor
// marks collected.

var (
	upiIDPattern     = regexp.MustCompile(`^[a-zA-Z0-9._-]{2,256}@[a-zA-Z]{2,64}$`)
	referencePattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)
	// paymentImageTypes are what a payment screenshot or a UPI QR may be.
	paymentImageTypes = []string{"image/jpeg", "image/png"}
)

// PaymentClaim is a direct UPI payment waiting for a check, with its booking.
type PaymentClaim struct {
	Payment     *models.Payment
	Appointment *models.AppointmentDetails
}

// ManualRefund is a refund of a direct UPI payment the doctor owes, with the
// payment and its booking.
type ManualRefund struct {
	Refund      *models.Refund
	Payment     *models.Payment
	Appointment *models.AppointmentDetails
}

func isManualProvider(provider string) bool {
	return provider == models.ProviderUPIDirect || provider == models.ProviderClinic
}

// enabled reports whether PAYMENT_METHODS turns method on.
func (ps *PaymentService) enabled(method models.PaymentMethod) bool {
	return slices.Contains(ps.methods, method)
}

// optionsFor lists how a booking with the doctor can be paid: razorpay and
// clinic when they are on, upi when it is on and the doctor gave a UPI ID. When
// neither online method can take the money, the clinic is the only way, whatever
// PAYMENT_METHODS says.
func (ps *PaymentService) optionsFor(doctor *models.Doctor) models.PaymentOptions {
	options := models.PaymentOptions{Methods: []models.PaymentMethod{}}
	if ps.enabled(models.PaymentMethodRazorpay) {
		options.Methods = append(options.Methods, models.PaymentMethodRazorpay)
	}
	if ps.enabled(models.PaymentMethodUPI) && doctor.UPIID != "" {
		options.Methods = append(options.Methods, models.PaymentMethodUPI)
		options.UPI = &models.DoctorUPI{
			UPIID:     doctor.UPIID,
			PayeeName: doctor.FullName,
			HasQR:     doctor.UPIQRStorageKey != nil,
		}
	}
	if ps.enabled(models.PaymentMethodClinic) || len(options.Methods) == 0 {
		options.Methods = append(options.Methods, models.PaymentMethodClinic)
	}

	return options
}

// bookingPayment is the payment state of a new booking with a fee: unpaid and
// holding its slot for constant.PaymentHold while an online method can settle
// it, due at the clinic when only the clinic can.
func (ps *PaymentService) bookingPayment(doctor *models.Doctor, now time.Time) (models.AppointmentPaymentStatus, *time.Time) {
	options := ps.optionsFor(doctor)
	if len(options.Methods) == 1 && options.Methods[0] == models.PaymentMethodClinic {
		return models.AppointmentPaymentDueAtClinic, nil
	}
	dueAt := now.Add(constant.PaymentHold)

	return models.AppointmentPaymentUnpaid, &dueAt
}

// Options returns how the patient side can pay an appointment, with the
// doctor's UPI details when upi is one of the ways.
func (ps *PaymentService) Options(ctx context.Context, actingUserID uint64, appointmentID uint64) (*models.PaymentOptions, error) {
	if actingUserID == 0 || appointmentID == 0 {
		return nil, fmt.Errorf("payment options with missing ids: %w", apperrors.ErrInvalidPaymentInput)
	}

	appointment, err := ps.payableAppointment(ctx, actingUserID, appointmentID)
	if err != nil {
		return nil, err
	}
	doctor, err := ps.doctors.GetByID(ctx, appointment.DoctorID)
	if err != nil {
		return nil, fmt.Errorf("get doctor of appointment: %w", err)
	}

	options := ps.optionsFor(doctor)

	return &options, nil
}

// StartUPI opens a direct UPI payment to the doctor for an unpaid booking and
// returns it with what the client shows: the UPI ID, payee name, amount and a
// upi:// link. An open payment by another method is closed first; asking again
// returns the same open UPI payment.
func (ps *PaymentService) StartUPI(
	ctx context.Context,
	actingUserID uint64,
	appointmentID uint64,
) (*models.Payment, map[string]any, error) {
	if actingUserID == 0 || appointmentID == 0 {
		return nil, nil, fmt.Errorf("start upi payment with missing ids: %w", apperrors.ErrInvalidPaymentInput)
	}

	appointment, err := ps.payableAppointment(ctx, actingUserID, appointmentID)
	if err != nil {
		return nil, nil, err
	}
	if err := ps.checkPayable(appointment); err != nil {
		return nil, nil, err
	}
	doctor, err := ps.doctors.GetByID(ctx, appointment.DoctorID)
	if err != nil {
		return nil, nil, fmt.Errorf("get doctor of appointment: %w", err)
	}
	if !slices.Contains(ps.optionsFor(doctor).Methods, models.PaymentMethodUPI) {
		return nil, nil, apperrors.ErrPaymentMethodUnavailable
	}

	existing, err := ps.openPayment(ctx, actingUserID, appointmentID, models.ProviderUPIDirect)
	if err != nil {
		return nil, nil, err
	}
	if existing != nil {
		return existing, upiPayload(existing, doctor), nil
	}

	created, err := ps.createManual(ctx, actingUserID, appointment, models.ProviderUPIDirect, models.PaymentStatusCreated, "")
	if err != nil {
		if errors.Is(err, apperrors.ErrPaymentAlreadyExists) {
			existing, getErr := ps.payments.GetActiveByAppointment(ctx, appointmentID)
			if getErr != nil {
				return nil, nil, fmt.Errorf("get payment created meanwhile: %w", getErr)
			}
			return existing, upiPayload(existing, doctor), nil
		}
		return nil, nil, err
	}
	ps.logger.InfoContext(ctx, "upi payment started", "payment_id", created.ID, "appointment_id", appointmentID, "amount", created.Amount)

	return created, upiPayload(created, doctor), nil
}

// PayAtClinic settles an unpaid booking as due at the clinic: it is confirmed
// now and the doctor collects the fee at the visit. An open online payment is
// closed.
func (ps *PaymentService) PayAtClinic(ctx context.Context, actingUserID uint64, appointmentID uint64) error {
	if actingUserID == 0 || appointmentID == 0 {
		return fmt.Errorf("pay at clinic with missing ids: %w", apperrors.ErrInvalidPaymentInput)
	}

	appointment, err := ps.payableAppointment(ctx, actingUserID, appointmentID)
	if err != nil {
		return err
	}
	if err := ps.checkPayable(appointment); err != nil {
		return err
	}
	doctor, err := ps.doctors.GetByID(ctx, appointment.DoctorID)
	if err != nil {
		return fmt.Errorf("get doctor of appointment: %w", err)
	}
	if !slices.Contains(ps.optionsFor(doctor).Methods, models.PaymentMethodClinic) {
		return apperrors.ErrPaymentMethodUnavailable
	}
	if _, err := ps.openPayment(ctx, actingUserID, appointmentID, ""); err != nil {
		return err
	}

	if err := ps.appointments.SetPaymentDue(ctx, appointmentID, models.AppointmentPaymentUnpaid, models.AppointmentPaymentDueAtClinic, nil); err != nil {
		if errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
			return apperrors.Invalid(apperrors.ErrPaymentWindowClosed, "", "the payment window for this booking has closed; book again")
		}
		return fmt.Errorf("set appointment due at clinic: %w", err)
	}
	ps.logger.InfoContext(ctx, "booking set to pay at clinic", "appointment_id", appointmentID)

	return nil
}

// Claim records the payer's word that they paid a direct UPI payment: their UPI
// reference (UTR) and an optional screenshot. It is accepted until the booking's
// payment was due; the slot then stays held until the start, waiting for the
// doctor's check.
func (ps *PaymentService) Claim(
	ctx context.Context,
	actingUserID uint64,
	paymentID uint64,
	reference string,
	proof io.Reader,
) (*models.Payment, error) {
	if actingUserID == 0 || paymentID == 0 {
		return nil, fmt.Errorf("claim payment with missing ids: %w", apperrors.ErrInvalidPaymentInput)
	}
	reference = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(reference), " ", ""))
	if len(reference) < constant.MinPayerReferenceLength || len(reference) > constant.MaxPayerReferenceLength || !referencePattern.MatchString(reference) {
		return nil, apperrors.Invalid(apperrors.ErrInvalidPaymentInput, "reference", fmt.Sprintf("must be the %d to %d letters or digits of the UPI reference", constant.MinPayerReferenceLength, constant.MaxPayerReferenceLength))
	}

	payment, err := ps.payments.GetByID(ctx, paymentID)
	if err != nil {
		return nil, err
	}
	if payment.Provider != models.ProviderUPIDirect {
		return nil, apperrors.ErrPaymentNotFound
	}
	appointment, err := ps.payableAppointment(ctx, actingUserID, payment.AppointmentID)
	if err != nil {
		if errors.Is(err, apperrors.ErrAppointmentNotFound) {
			return nil, apperrors.ErrPaymentNotFound
		}
		return nil, err
	}
	if payment.Status != models.PaymentStatusCreated {
		return nil, apperrors.Invalid(apperrors.ErrPaymentNotModifiable, "", fmt.Sprintf("payment is %s", payment.Status))
	}
	if err := ps.checkPayable(appointment); err != nil {
		return nil, err
	}

	var proofKey *string
	if proof != nil {
		key, err := ps.storeImage(ctx, fmt.Sprintf("payments/%d/proof", payment.ID), "proof", proof)
		if err != nil {
			return nil, err
		}
		proofKey = &key
	}

	claimedAt := ps.now()
	startsAt := appointment.StartsAt
	err = ps.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := ps.payments.Claim(txCtx, payment.ID, reference, proofKey, claimedAt); err != nil {
			return err
		}
		if err := ps.appointments.SetPaymentDue(txCtx, appointment.ID, models.AppointmentPaymentUnpaid, models.AppointmentPaymentPendingVerification, &startsAt); err != nil {
			return err
		}
		if err := ps.payments.CreateStatusChange(txCtx, &models.PaymentStatusChange{
			PaymentID:       payment.ID,
			ChangedByUserID: actingUserID,
			FromStatus:      models.PaymentStatusCreated,
			ToStatus:        models.PaymentStatusClaimed,
			Reason:          "payer says they paid by upi",
		}); err != nil {
			return err
		}

		claimed := *payment
		claimed.Status = models.PaymentStatusClaimed
		return ps.outbox.Create(txCtx, paymentEvent(models.OutboxEventPaymentClaimed, &claimed))
	})
	if err != nil {
		if proofKey != nil {
			_ = ps.storage.Delete(ctx, *proofKey)
		}
		if errors.Is(err, apperrors.ErrPaymentNotModifiable) || errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
			return nil, apperrors.Invalid(apperrors.ErrPaymentWindowClosed, "", "the payment window for this booking has closed; book again")
		}
		return nil, fmt.Errorf("claim payment: %w", err)
	}
	ps.logger.InfoContext(ctx, "upi payment claimed", "payment_id", payment.ID, "appointment_id", appointment.ID)

	return ps.payments.GetByID(ctx, payment.ID)
}

// VerifyClaim confirms that a claimed direct UPI payment arrived: the payment is
// captured and the booking paid. The appointment's doctor checks it, or an admin.
func (ps *PaymentService) VerifyClaim(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	paymentID uint64,
	note string,
) (*models.Payment, error) {
	payment, appointment, note, err := ps.claimToReview(ctx, actingUserID, actingRole, paymentID, note)
	if err != nil {
		return nil, err
	}

	err = ps.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := ps.payments.Review(txCtx, payment.ID, models.PaymentStatusCaptured, actingUserID, note, ps.now()); err != nil {
			return err
		}
		if err := ps.appointments.SetPaymentDue(txCtx, appointment.ID, models.AppointmentPaymentPendingVerification, models.AppointmentPaymentPaid, nil); err != nil {
			return err
		}
		if err := ps.payments.CreateStatusChange(txCtx, &models.PaymentStatusChange{
			PaymentID:       payment.ID,
			ChangedByUserID: actingUserID,
			FromStatus:      models.PaymentStatusClaimed,
			ToStatus:        models.PaymentStatusCaptured,
			Reason:          reviewReason("upi payment verified", note),
		}); err != nil {
			return err
		}

		captured := *payment
		captured.Status = models.PaymentStatusCaptured
		return ps.outbox.Create(txCtx, paymentEvent(models.OutboxEventPaymentCaptured, &captured))
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrPaymentNotModifiable) || errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
			return nil, apperrors.Invalid(apperrors.ErrPaymentNotModifiable, "", "this payment is no longer waiting for a check")
		}
		return nil, fmt.Errorf("verify payment: %w", err)
	}
	ps.logger.InfoContext(ctx, "upi payment verified", "payment_id", payment.ID, "appointment_id", appointment.ID, "role", actingRole)

	return ps.payments.GetByID(ctx, payment.ID)
}

// RejectClaim records that a claimed direct UPI payment never arrived: the
// payment is rejected and the booking cancelled with reason payment_rejected,
// freeing its slot. reason is shown to the patient.
func (ps *PaymentService) RejectClaim(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	paymentID uint64,
	reason string,
) (*models.Payment, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, apperrors.Invalid(apperrors.ErrInvalidPaymentInput, "reason", "is required")
	}
	payment, appointment, reason, err := ps.claimToReview(ctx, actingUserID, actingRole, paymentID, reason)
	if err != nil {
		return nil, err
	}

	err = ps.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := ps.payments.Review(txCtx, payment.ID, models.PaymentStatusRejected, actingUserID, reason, ps.now()); err != nil {
			return err
		}
		if err := ps.appointments.CancelForRejectedPayment(txCtx, appointment.ID, actingUserID); err != nil {
			return err
		}
		if err := ps.appointments.CreateStatusChange(txCtx, &models.AppointmentStatusChange{
			AppointmentID:   appointment.ID,
			ChangedByUserID: actingUserID,
			FromStatus:      models.AppointmentStatusScheduled,
			ToStatus:        models.AppointmentStatusCancelled,
			Reason:          reviewReason("upi payment not received", reason),
		}); err != nil {
			return err
		}
		if err := ps.payments.CreateStatusChange(txCtx, &models.PaymentStatusChange{
			PaymentID:       payment.ID,
			ChangedByUserID: actingUserID,
			FromStatus:      models.PaymentStatusClaimed,
			ToStatus:        models.PaymentStatusRejected,
			Reason:          reviewReason("upi payment not received", reason),
		}); err != nil {
			return err
		}

		rejected := *payment
		rejected.Status = models.PaymentStatusRejected
		if err := ps.outbox.Create(txCtx, paymentEvent(models.OutboxEventPaymentRejected, &rejected)); err != nil {
			return err
		}
		cancelled := appointment.Appointment
		cancelled.Status = models.AppointmentStatusCancelled
		cancelled.CancelReason = models.CancelReasonPaymentRejected
		return ps.outbox.Create(txCtx, appointmentEvent(models.OutboxEventAppointmentCancelled, &cancelled, actingUserID))
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrPaymentNotModifiable) || errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
			return nil, apperrors.Invalid(apperrors.ErrPaymentNotModifiable, "", "this payment is no longer waiting for a check")
		}
		return nil, fmt.Errorf("reject payment: %w", err)
	}
	ps.logger.InfoContext(ctx, "upi payment rejected", "payment_id", payment.ID, "appointment_id", appointment.ID, "role", actingRole)

	return ps.payments.GetByID(ctx, payment.ID)
}

// ListClaims returns the direct UPI payments waiting for a check, oldest first,
// with their bookings: the acting doctor's own, or every one for an admin.
func (ps *PaymentService) ListClaims(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	limit int,
	offset int,
) ([]*PaymentClaim, error) {
	doctorID, err := ps.reviewScope(ctx, actingUserID, actingRole)
	if err != nil {
		return nil, err
	}
	limit, offset = clampPagination(limit, offset)

	payments, err := ps.payments.ListClaimed(ctx, doctorID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list claimed payments: %w", err)
	}

	claims := make([]*PaymentClaim, 0, len(payments))
	for _, payment := range payments {
		appointment, err := ps.appointments.GetByID(ctx, payment.AppointmentID)
		if err != nil {
			return nil, fmt.Errorf("get appointment of payment %d: %w", payment.ID, err)
		}
		claims = append(claims, &PaymentClaim{Payment: payment, Appointment: appointment})
	}

	return claims, nil
}

// MarkCollected records that the doctor collected a booking's fee at the clinic:
// a clinic payment is captured and the booking paid. It works on a booking due
// at the clinic that is scheduled or completed.
func (ps *PaymentService) MarkCollected(
	ctx context.Context,
	actingUserID uint64,
	appointmentID uint64,
	note string,
) (*models.Payment, error) {
	if actingUserID == 0 || appointmentID == 0 {
		return nil, fmt.Errorf("mark collected with missing ids: %w", apperrors.ErrInvalidPaymentInput)
	}
	note, err := reviewNote(note)
	if err != nil {
		return nil, err
	}

	appointment, err := ps.appointments.GetByID(ctx, appointmentID)
	if err != nil {
		return nil, err
	}
	if err := ps.checkDoctorOf(ctx, actingUserID, models.RoleDoctor, appointment); err != nil {
		if errors.Is(err, apperrors.ErrPaymentNotFound) {
			return nil, apperrors.ErrAppointmentNotFound
		}
		return nil, err
	}
	if appointment.PaymentStatus != models.AppointmentPaymentDueAtClinic {
		return nil, apperrors.Invalid(apperrors.ErrPaymentNotRequired, "", fmt.Sprintf("appointment payment is %s", appointment.PaymentStatus))
	}
	if appointment.Status != models.AppointmentStatusScheduled && appointment.Status != models.AppointmentStatusCompleted {
		return nil, apperrors.Invalid(apperrors.ErrAppointmentNotModifiable, "", fmt.Sprintf("appointment is %s", appointment.Status))
	}

	collected, err := ps.createManual(ctx, actingUserID, appointment, models.ProviderClinic, models.PaymentStatusCaptured, note)
	if err != nil {
		if errors.Is(err, apperrors.ErrAppointmentNotModifiable) || errors.Is(err, apperrors.ErrPaymentAlreadyExists) {
			return nil, apperrors.Invalid(apperrors.ErrPaymentNotRequired, "", "the fee was already collected")
		}
		return nil, err
	}
	ps.logger.InfoContext(ctx, "clinic fee collected", "payment_id", collected.ID, "appointment_id", appointmentID)

	return collected, nil
}

// ListManualRefunds returns refunds of direct UPI payments still to be made,
// oldest first: those the acting doctor owes, or every one for an admin.
func (ps *PaymentService) ListManualRefunds(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	limit int,
	offset int,
) ([]*ManualRefund, error) {
	doctorID, err := ps.reviewScope(ctx, actingUserID, actingRole)
	if err != nil {
		return nil, err
	}
	limit, offset = clampPagination(limit, offset)

	refunds, err := ps.payments.ListManualRefunds(ctx, doctorID, models.RefundStatusRequested, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list manual refunds: %w", err)
	}

	result := make([]*ManualRefund, 0, len(refunds))
	for _, refund := range refunds {
		payment, err := ps.payments.GetByID(ctx, refund.PaymentID)
		if err != nil {
			return nil, fmt.Errorf("get payment of refund %d: %w", refund.ID, err)
		}
		appointment, err := ps.appointments.GetByID(ctx, payment.AppointmentID)
		if err != nil {
			return nil, fmt.Errorf("get appointment of refund %d: %w", refund.ID, err)
		}
		result = append(result, &ManualRefund{Refund: refund, Payment: payment, Appointment: appointment})
	}

	return result, nil
}

// CloseManualRefund settles a refund of a direct UPI payment by hand. refunded
// true: the doctor sent the money back, and the payment and booking are
// refunded. refunded false, only for a payment that was never checked: the money
// never arrived, so there is nothing to return and the claim is rejected.
func (ps *PaymentService) CloseManualRefund(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	refundID uint64,
	refunded bool,
	note string,
) error {
	if actingUserID == 0 || refundID == 0 {
		return fmt.Errorf("close refund with missing ids: %w", apperrors.ErrInvalidPaymentInput)
	}
	note, err := reviewNote(note)
	if err != nil {
		return err
	}

	refund, err := ps.payments.GetRefundByID(ctx, refundID)
	if err != nil {
		return err
	}
	payment, err := ps.payments.GetByID(ctx, refund.PaymentID)
	if err != nil {
		return fmt.Errorf("get payment of refund: %w", err)
	}
	if payment.Provider != models.ProviderUPIDirect {
		return apperrors.ErrRefundNotFound
	}
	appointment, err := ps.appointments.GetByID(ctx, payment.AppointmentID)
	if err != nil {
		return fmt.Errorf("get appointment of refund: %w", err)
	}
	if err := ps.checkDoctorOf(ctx, actingUserID, actingRole, appointment); err != nil {
		if errors.Is(err, apperrors.ErrPaymentNotFound) {
			return apperrors.ErrRefundNotFound
		}
		return err
	}
	if refund.Status != models.RefundStatusRequested {
		return apperrors.Invalid(apperrors.ErrPaymentNotModifiable, "", fmt.Sprintf("refund is %s", refund.Status))
	}

	if refunded {
		if err := ps.finishRefund(ctx, payment, refund.ID, "manual", refund.Reason, reviewReason("refunded by upi", note), actingUserID); err != nil {
			return fmt.Errorf("finish manual refund: %w", err)
		}
		ps.logger.InfoContext(ctx, "manual refund settled", "refund_id", refund.ID, "payment_id", payment.ID, "role", actingRole)
		return nil
	}

	if payment.Status != models.PaymentStatusClaimed {
		return apperrors.Invalid(apperrors.ErrPaymentNotModifiable, "", "the payment was verified as received, so it must be refunded")
	}
	err = ps.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := ps.payments.FinishRefund(txCtx, refund.ID, models.RefundStatusFailed, "not_received"); err != nil {
			return err
		}
		if err := ps.payments.Review(txCtx, payment.ID, models.PaymentStatusRejected, actingUserID, note, ps.now()); err != nil {
			return err
		}
		if err := ps.payments.CreateStatusChange(txCtx, &models.PaymentStatusChange{
			PaymentID:       payment.ID,
			ChangedByUserID: actingUserID,
			FromStatus:      models.PaymentStatusClaimed,
			ToStatus:        models.PaymentStatusRejected,
			Reason:          reviewReason("upi payment never received", note),
		}); err != nil {
			return err
		}
		if err := ps.appointments.SetPaymentStatus(txCtx, appointment.ID, models.AppointmentPaymentPendingVerification, models.AppointmentPaymentUnpaid); err != nil &&
			!errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
			return err
		}

		rejected := *payment
		rejected.Status = models.PaymentStatusRejected
		return ps.outbox.Create(txCtx, paymentEvent(models.OutboxEventPaymentRejected, &rejected))
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrPaymentNotModifiable) {
			return apperrors.Invalid(apperrors.ErrPaymentNotModifiable, "", "this refund was already closed")
		}
		return fmt.Errorf("close refund as not received: %w", err)
	}
	ps.logger.InfoContext(ctx, "manual refund closed as not received", "refund_id", refund.ID, "payment_id", payment.ID, "role", actingRole)

	return nil
}

// SetDoctorUPI sets the UPI ID patients pay the acting doctor at; empty stops
// direct UPI payments to them (the QR goes too).
func (ps *PaymentService) SetDoctorUPI(ctx context.Context, actingUserID uint64, actingRole models.Role, upiID string) (*models.Doctor, error) {
	doctor, err := ps.ownDoctor(ctx, actingUserID, actingRole)
	if err != nil {
		return nil, err
	}
	upiID = strings.TrimSpace(upiID)
	if upiID != "" && (len(upiID) > constant.MaxUPIIDLength || !upiIDPattern.MatchString(upiID)) {
		return nil, apperrors.Invalid(apperrors.ErrInvalidDoctorInput, "upiId", "must look like name@bank")
	}

	qrKey := doctor.UPIQRStorageKey
	if upiID == "" {
		qrKey = nil
	}
	if err := ps.doctors.UpdateUPI(ctx, doctor.ID, upiID, qrKey); err != nil {
		return nil, fmt.Errorf("update doctor upi: %w", err)
	}
	if upiID == "" && doctor.UPIQRStorageKey != nil {
		_ = ps.storage.Delete(ctx, *doctor.UPIQRStorageKey)
	}
	ps.logger.InfoContext(ctx, "doctor upi updated", "doctor_id", doctor.ID, "enabled", upiID != "")

	return ps.doctors.GetByID(ctx, doctor.ID)
}

// SetDoctorUPIQR stores the acting doctor's UPI QR image, a JPEG or PNG,
// replacing the old one. The UPI ID must be set first.
func (ps *PaymentService) SetDoctorUPIQR(ctx context.Context, actingUserID uint64, actingRole models.Role, content io.Reader) (*models.Doctor, error) {
	doctor, err := ps.ownDoctor(ctx, actingUserID, actingRole)
	if err != nil {
		return nil, err
	}
	if doctor.UPIID == "" {
		return nil, apperrors.Invalid(apperrors.ErrInvalidDoctorInput, "upiId", "set your UPI ID before adding a QR")
	}
	if content == nil {
		return nil, apperrors.Invalid(apperrors.ErrInvalidDoctorInput, "file", "is required")
	}

	key, err := ps.storeImage(ctx, fmt.Sprintf("doctors/%d/upi-qr", doctor.ID), "file", content)
	if err != nil {
		return nil, err
	}
	if err := ps.doctors.UpdateUPI(ctx, doctor.ID, doctor.UPIID, &key); err != nil {
		_ = ps.storage.Delete(ctx, key)
		return nil, fmt.Errorf("save doctor upi qr: %w", err)
	}
	if doctor.UPIQRStorageKey != nil {
		_ = ps.storage.Delete(ctx, *doctor.UPIQRStorageKey)
	}
	ps.logger.InfoContext(ctx, "doctor upi qr updated", "doctor_id", doctor.ID)

	return ps.doctors.GetByID(ctx, doctor.ID)
}

// DeleteDoctorUPIQR removes the acting doctor's UPI QR image.
func (ps *PaymentService) DeleteDoctorUPIQR(ctx context.Context, actingUserID uint64, actingRole models.Role) (*models.Doctor, error) {
	doctor, err := ps.ownDoctor(ctx, actingUserID, actingRole)
	if err != nil {
		return nil, err
	}
	if doctor.UPIQRStorageKey == nil {
		return doctor, nil
	}
	if err := ps.doctors.UpdateUPI(ctx, doctor.ID, doctor.UPIID, nil); err != nil {
		return nil, fmt.Errorf("remove doctor upi qr: %w", err)
	}
	_ = ps.storage.Delete(ctx, *doctor.UPIQRStorageKey)

	return ps.doctors.GetByID(ctx, doctor.ID)
}

// OpenUPIQR streams the UPI QR of an appointment's doctor to its patient side,
// or the acting doctor's own when appointmentID is 0.
func (ps *PaymentService) OpenUPIQR(ctx context.Context, actingUserID uint64, actingRole models.Role, appointmentID uint64) (io.ReadCloser, error) {
	var doctor *models.Doctor
	var err error
	if appointmentID == 0 {
		doctor, err = ps.ownDoctor(ctx, actingUserID, actingRole)
	} else {
		var appointment *models.AppointmentDetails
		appointment, err = ps.payableAppointment(ctx, actingUserID, appointmentID)
		if err == nil {
			doctor, err = ps.doctors.GetByID(ctx, appointment.DoctorID)
		}
	}
	if err != nil {
		return nil, err
	}
	if doctor.UPIQRStorageKey == nil {
		return nil, apperrors.ErrPaymentImageNotFound
	}

	return ps.openImage(ctx, *doctor.UPIQRStorageKey)
}

// OpenProof streams a direct UPI payment's screenshot to the appointment's
// doctor, an admin or the patient side.
func (ps *PaymentService) OpenProof(ctx context.Context, actingUserID uint64, actingRole models.Role, paymentID uint64) (io.ReadCloser, error) {
	payment, err := ps.payments.GetByID(ctx, paymentID)
	if err != nil {
		return nil, err
	}
	appointment, err := ps.appointments.GetByID(ctx, payment.AppointmentID)
	if err != nil {
		return nil, fmt.Errorf("get appointment of payment: %w", err)
	}
	if err := ps.checkCanSee(ctx, actingUserID, actingRole, appointment); err != nil {
		if errors.Is(err, apperrors.ErrAppointmentNotFound) {
			return nil, apperrors.ErrPaymentNotFound
		}
		return nil, err
	}
	if payment.ProofStorageKey == nil {
		return nil, apperrors.ErrPaymentImageNotFound
	}

	return ps.openImage(ctx, *payment.ProofStorageKey)
}

// openPayment returns the appointment's open payment when it is by provider,
// and expires an open one by any other method, so the patient can switch
// methods before paying. An empty provider expires whatever is open.
func (ps *PaymentService) openPayment(ctx context.Context, actingUserID uint64, appointmentID uint64, provider string) (*models.Payment, error) {
	existing, err := ps.payments.GetActiveByAppointment(ctx, appointmentID)
	if errors.Is(err, apperrors.ErrPaymentNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active payment: %w", err)
	}
	if existing.Status != models.PaymentStatusCreated {
		return nil, apperrors.Invalid(apperrors.ErrPaymentNotRequired, "", fmt.Sprintf("a payment is already %s for this booking", existing.Status))
	}
	if provider != "" && existing.Provider == provider {
		return existing, nil
	}

	err = ps.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := ps.payments.UpdateStatus(txCtx, existing.ID, models.PaymentStatusCreated, models.PaymentStatusExpired, "", nil); err != nil {
			return err
		}

		return ps.payments.CreateStatusChange(txCtx, &models.PaymentStatusChange{
			PaymentID:       existing.ID,
			ChangedByUserID: actingUserID,
			FromStatus:      models.PaymentStatusCreated,
			ToStatus:        models.PaymentStatusExpired,
			Reason:          "payer chose another payment method",
		})
	})
	if err != nil && !errors.Is(err, apperrors.ErrPaymentNotModifiable) {
		return nil, fmt.Errorf("close payment by another method: %w", err)
	}

	return nil, nil
}

// createManual records a payment that moves no money through the gateway. A
// captured one (a fee collected at the clinic) also marks the booking paid in
// the same transaction.
func (ps *PaymentService) createManual(
	ctx context.Context,
	actingUserID uint64,
	appointment *models.AppointmentDetails,
	provider string,
	status models.PaymentStatus,
	note string,
) (*models.Payment, error) {
	token, err := utils.NewOpaqueToken()
	if err != nil {
		return nil, fmt.Errorf("create manual order id: %w", err)
	}
	payment := &models.Payment{
		AppointmentID:   appointment.ID,
		PayerUserID:     actingUserID,
		Amount:          appointment.ConsultationFee,
		Currency:        constant.PaymentCurrency,
		Provider:        provider,
		ProviderOrderID: fmt.Sprintf("%s_%d_%s", provider, appointment.ID, token[:12]),
		Status:          status,
	}
	if status == models.PaymentStatusCaptured {
		now := ps.now()
		payment.CapturedAt = &now
		payment.ReviewedByUserID = actingUserID
		payment.ReviewedAt = &now
		payment.ReviewNote = note
	}

	var created *models.Payment
	err = ps.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		created, err = ps.payments.Create(txCtx, payment)
		if err != nil {
			return err
		}
		if err := ps.payments.CreateStatusChange(txCtx, &models.PaymentStatusChange{
			PaymentID:       created.ID,
			ChangedByUserID: actingUserID,
			ToStatus:        status,
			Reason:          reviewReason("", note),
		}); err != nil {
			return err
		}
		if status != models.PaymentStatusCaptured {
			return nil
		}
		if err := ps.appointments.SetPaymentStatus(txCtx, appointment.ID, models.AppointmentPaymentDueAtClinic, models.AppointmentPaymentPaid); err != nil {
			return err
		}

		return ps.outbox.Create(txCtx, paymentEvent(models.OutboxEventPaymentCaptured, created))
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrPaymentAlreadyExists) || errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
			return nil, err
		}
		return nil, fmt.Errorf("create %s payment: %w", provider, err)
	}

	return created, nil
}

// claimToReview loads a claimed direct UPI payment for the appointment's doctor
// or an admin, with its booking and the cleaned note.
func (ps *PaymentService) claimToReview(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	paymentID uint64,
	note string,
) (*models.Payment, *models.AppointmentDetails, string, error) {
	if actingUserID == 0 || paymentID == 0 {
		return nil, nil, "", fmt.Errorf("review payment with missing ids: %w", apperrors.ErrInvalidPaymentInput)
	}
	note, err := reviewNote(note)
	if err != nil {
		return nil, nil, "", err
	}

	payment, err := ps.payments.GetByID(ctx, paymentID)
	if err != nil {
		return nil, nil, "", err
	}
	if payment.Provider != models.ProviderUPIDirect {
		return nil, nil, "", apperrors.ErrPaymentNotFound
	}
	appointment, err := ps.appointments.GetByID(ctx, payment.AppointmentID)
	if err != nil {
		return nil, nil, "", fmt.Errorf("get appointment of payment: %w", err)
	}
	if err := ps.checkDoctorOf(ctx, actingUserID, actingRole, appointment); err != nil {
		return nil, nil, "", err
	}
	if payment.Status != models.PaymentStatusClaimed {
		return nil, nil, "", apperrors.Invalid(apperrors.ErrPaymentNotModifiable, "", fmt.Sprintf("payment is %s, not waiting for a check", payment.Status))
	}

	return payment, appointment, note, nil
}

// checkDoctorOf lets an admin, or the appointment's own doctor, act on its
// payment; anyone else reads the payment as not found.
func (ps *PaymentService) checkDoctorOf(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointment *models.AppointmentDetails,
) error {
	if actingRole == models.RoleAdmin {
		return nil
	}
	if actingRole == models.RoleDoctor {
		doctor, err := ps.doctors.GetByUserID(ctx, actingUserID)
		if err != nil && !errors.Is(err, apperrors.ErrDoctorNotFound) {
			return fmt.Errorf("get own doctor profile: %w", err)
		}
		if err == nil && doctor.ID == appointment.DoctorID {
			return nil
		}
	}

	return apperrors.ErrPaymentNotFound
}

// reviewScope is the doctor whose claims and refunds a list shows: the acting
// doctor's, or 0 (every doctor) for an admin.
func (ps *PaymentService) reviewScope(ctx context.Context, actingUserID uint64, actingRole models.Role) (uint64, error) {
	if actingRole == models.RoleAdmin {
		return 0, nil
	}
	doctor, err := ps.ownDoctor(ctx, actingUserID, actingRole)
	if err != nil {
		return 0, err
	}

	return doctor.ID, nil
}

func (ps *PaymentService) ownDoctor(ctx context.Context, actingUserID uint64, actingRole models.Role) (*models.Doctor, error) {
	if err := validateDoctorActor(actingUserID, actingRole); err != nil {
		return nil, err
	}
	doctor, err := ps.doctors.GetByUserID(ctx, actingUserID)
	if err != nil {
		if errors.Is(err, apperrors.ErrDoctorNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get own doctor profile: %w", err)
	}

	return doctor, nil
}

// storeImage stores a JPEG or PNG of at most constant.MaxPaymentImageBytes
// under a new key starting with prefix and returns the key. field names the
// request field in validation reasons.
func (ps *PaymentService) storeImage(ctx context.Context, prefix string, field string, content io.Reader) (string, error) {
	head := make([]byte, constant.ContentSniffBytes)
	read, err := io.ReadFull(content, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read image: %w", err)
	}
	if read == 0 {
		return "", apperrors.Invalid(apperrors.ErrInvalidPaymentInput, field, "is empty")
	}
	contentType := http.DetectContentType(head[:read])
	if !slices.Contains(paymentImageTypes, contentType) {
		return "", apperrors.Invalid(apperrors.ErrInvalidPaymentInput, field, "must be a JPEG or PNG image")
	}

	token, err := utils.NewOpaqueToken()
	if err != nil {
		return "", fmt.Errorf("create storage key: %w", err)
	}
	key := fmt.Sprintf("%s-%s", prefix, token)

	counted := &countingReader{reader: io.LimitReader(io.MultiReader(bytes.NewReader(head[:read]), content), constant.MaxPaymentImageBytes+1)}
	if err := ps.storage.Put(ctx, key, counted, contentType); err != nil {
		return "", fmt.Errorf("store image: %w", err)
	}
	if counted.read > constant.MaxPaymentImageBytes {
		_ = ps.storage.Delete(ctx, key)
		return "", apperrors.Invalid(apperrors.ErrInvalidPaymentInput, field, fmt.Sprintf("must be at most %d MB", constant.MaxPaymentImageBytes>>20))
	}

	return key, nil
}

func (ps *PaymentService) openImage(ctx context.Context, key string) (io.ReadCloser, error) {
	file, err := ps.storage.Open(ctx, key)
	if err != nil {
		if errors.Is(err, apperrors.ErrStoredFileNotFound) {
			return nil, apperrors.ErrPaymentImageNotFound
		}
		return nil, fmt.Errorf("open payment image: %w", err)
	}

	return file, nil
}

// upiPayload is what the client shows to pay a direct UPI payment. The link is
// the standard upi:// intent with the amount fixed, which UPI apps open.
func upiPayload(payment *models.Payment, doctor *models.Doctor) map[string]any {
	amount := fmt.Sprintf("%d.%02d", payment.Amount/100, payment.Amount%100)
	note := fmt.Sprintf("MedAppoint booking %d", payment.AppointmentID)
	query := url.Values{}
	query.Set("pa", doctor.UPIID)
	query.Set("pn", doctor.FullName)
	query.Set("am", amount)
	query.Set("cu", constant.PaymentCurrency)
	query.Set("tn", note)

	return map[string]any{
		"upiId":     doctor.UPIID,
		"payeeName": doctor.FullName,
		"amount":    amount,
		"note":      note,
		"upiLink":   "upi://pay?" + strings.ReplaceAll(query.Encode(), "+", "%20"),
		"hasQr":     doctor.UPIQRStorageKey != nil,
	}
}

func reviewNote(note string) (string, error) {
	note = strings.TrimSpace(note)
	if len(note) > constant.MaxReviewNoteLength {
		return "", apperrors.Invalid(apperrors.ErrInvalidPaymentInput, "note", fmt.Sprintf("must be at most %d characters", constant.MaxReviewNoteLength))
	}

	return note, nil
}

// reviewReason joins what happened and the person's note for a history row.
func reviewReason(what string, note string) string {
	switch {
	case what == "":
		return note
	case note == "":
		return what
	default:
		return what + ": " + note
	}
}
