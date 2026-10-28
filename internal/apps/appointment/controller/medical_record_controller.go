package controller

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/dtos/record"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"context"
	"io"
	"time"
)

type MedicalRecordController struct {
	medicalRecordService *services.MedicalRecordService
}

func NewMedicalRecordController(medicalRecordService *services.MedicalRecordService) (*MedicalRecordController, error) {
	if medicalRecordService == nil {
		return nil, apperrors.ErrNilMedicalRecordService
	}
	return &MedicalRecordController{
		medicalRecordService: medicalRecordService,
	}, nil
}

func (mrc *MedicalRecordController) CreateDraft(ctx context.Context, request *record.CreateRecordRequest) (*record.MedicalRecordResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	createdRecord, err := mrc.medicalRecordService.CreateDraft(ctx, userID, role, request.AppointmentID)
	if err != nil {
		return nil, err
	}

	response := toMedicalRecordResponse(createdRecord, nil)
	return &response, nil
}

func (mrc *MedicalRecordController) UpdateDraft(ctx context.Context, request *record.UpdateRecordRequest) (*record.MedicalRecordResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	changedRecord := models.MedicalRecord{
		ID:                request.ID,
		ChiefComplaint:    request.ChiefComplaint,
		Symptoms:          request.Symptoms,
		Diagnosis:         request.Diagnosis,
		ExaminationNotes:  request.ExaminationNotes,
		Advice:            request.Advice,
		PrescriptionNotes: request.PrescriptionNotes,
		Vitals: models.Vitals{
			BloodPressure:      request.Vitals.BloodPressure,
			PulsePerMinute:     request.Vitals.PulsePerMinute,
			TemperatureCelsius: request.Vitals.TemperatureCelsius,
			WeightKg:           request.Vitals.WeightKg,
			SpO2Percent:        request.Vitals.SpO2Percent,
		},
		Medicines: make([]models.PrescriptionItem, 0, len(request.Medicines)),
	}
	if request.FollowUpOn != "" {
		followUpOn, err := time.Parse(constant.DateLayout, request.FollowUpOn)
		if err != nil {
			return nil, apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, "followUpOn", "must be YYYY-MM-DD")
		}
		changedRecord.FollowUpOn = &followUpOn
	}
	for _, medicine := range request.Medicines {
		changedRecord.Medicines = append(changedRecord.Medicines, models.PrescriptionItem{
			MedicineName: medicine.MedicineName,
			Strength:     medicine.Strength,
			Form:         medicine.Form,
			Dosage:       medicine.Dosage,
			Frequency:    medicine.Frequency,
			DurationDays: medicine.DurationDays,
			Timing:       models.MedicineTiming(medicine.Timing),
			Instructions: medicine.Instructions,
		})
	}

	updatedRecord, err := mrc.medicalRecordService.UpdateDraft(ctx, userID, role, &changedRecord)
	if err != nil {
		return nil, err
	}

	response := toMedicalRecordResponse(updatedRecord, nil)
	return &response, nil
}

func (mrc *MedicalRecordController) Finalize(ctx context.Context, request *record.FinalizeRecordRequest) (*record.MedicalRecordResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	finalizedRecord, err := mrc.medicalRecordService.Finalize(ctx, userID, role, request.ID)
	if err != nil {
		return nil, err
	}

	response := toMedicalRecordResponse(finalizedRecord, nil)
	return &response, nil
}

func (mrc *MedicalRecordController) GetByID(ctx context.Context, request *record.GetRecordRequest) (*record.MedicalRecordResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	foundRecord, attachments, err := mrc.medicalRecordService.GetByID(ctx, userID, role, request.ID)
	if err != nil {
		return nil, err
	}

	response := toMedicalRecordResponse(foundRecord, attachments)
	return &response, nil
}

func (mrc *MedicalRecordController) ListForPatient(ctx context.Context, request *record.ListRecordsRequest) (*record.ListRecordsResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	records, err := mrc.medicalRecordService.ListForPatient(ctx, userID, request.PatientID, request.Limit, request.Offset)
	if err != nil {
		return nil, err
	}

	response := record.ListRecordsResponse{
		Records: make([]record.MedicalRecordResponse, 0, len(records)),
	}
	for _, listedRecord := range records {
		response.Records = append(response.Records, toMedicalRecordResponse(listedRecord, nil))
	}

	return &response, nil
}

// ListForDoctor lists what a doctor treating the patient may read.
func (mrc *MedicalRecordController) ListForDoctor(ctx context.Context, request *record.ListRecordsRequest) (*record.ListRecordsResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	records, err := mrc.medicalRecordService.ListForDoctor(ctx, userID, role, request.PatientID, request.Limit, request.Offset)
	if err != nil {
		return nil, err
	}

	response := record.ListRecordsResponse{
		Records: make([]record.MedicalRecordResponse, 0, len(records)),
	}
	for _, listedRecord := range records {
		response.Records = append(response.Records, toMedicalRecordResponse(listedRecord, nil))
	}

	return &response, nil
}

// UploadForPatient stores a file the patient side adds outside a visit.
func (mrc *MedicalRecordController) UploadForPatient(ctx context.Context, request *record.UploadAttachmentRequest, file io.Reader) (*record.AttachmentResponse, error) {
	attachment := models.MedicalAttachment{PatientID: request.ID}
	return mrc.upload(ctx, request, &attachment, file)
}

// UploadForRecord stores a file the doctor adds to their draft record.
func (mrc *MedicalRecordController) UploadForRecord(ctx context.Context, request *record.UploadAttachmentRequest, file io.Reader) (*record.AttachmentResponse, error) {
	recordID := request.ID
	attachment := models.MedicalAttachment{MedicalRecordID: &recordID}
	return mrc.upload(ctx, request, &attachment, file)
}

func (mrc *MedicalRecordController) ListAttachments(ctx context.Context, request *record.ListAttachmentsRequest) (*record.ListAttachmentsResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	attachments, err := mrc.medicalRecordService.ListAttachments(ctx, userID, request.PatientID, request.Limit, request.Offset)
	if err != nil {
		return nil, err
	}

	response := record.ListAttachmentsResponse{
		Attachments: make([]record.AttachmentResponse, 0, len(attachments)),
	}
	for _, attachment := range attachments {
		response.Attachments = append(response.Attachments, toAttachmentResponse(attachment))
	}

	return &response, nil
}

func (mrc *MedicalRecordController) ListAttachmentsForDoctor(ctx context.Context, request *record.ListAttachmentsRequest) (*record.ListAttachmentsResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	attachments, err := mrc.medicalRecordService.ListAttachmentsForDoctor(ctx, userID, role, request.PatientID, request.Limit, request.Offset)
	if err != nil {
		return nil, err
	}

	response := record.ListAttachmentsResponse{
		Attachments: make([]record.AttachmentResponse, 0, len(attachments)),
	}
	for _, attachment := range attachments {
		response.Attachments = append(response.Attachments, toAttachmentResponse(attachment))
	}

	return &response, nil
}

// OpenAttachment returns the attachment and its file; the caller closes the file.
func (mrc *MedicalRecordController) OpenAttachment(ctx context.Context, request *record.GetAttachmentRequest) (*record.AttachmentResponse, io.ReadCloser, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, nil, err
	}

	attachment, file, err := mrc.medicalRecordService.OpenAttachment(ctx, userID, role, request.ID)
	if err != nil {
		return nil, nil, err
	}

	response := toAttachmentResponse(attachment)
	return &response, file, nil
}

func (mrc *MedicalRecordController) upload(
	ctx context.Context,
	request *record.UploadAttachmentRequest,
	attachment *models.MedicalAttachment,
	file io.Reader,
) (*record.AttachmentResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	attachment.Kind = models.AttachmentKind(request.Kind)
	attachment.Title = request.Title
	if request.TakenOn != "" {
		takenOn, err := time.Parse(constant.DateLayout, request.TakenOn)
		if err != nil {
			return nil, apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, "takenOn", "must be YYYY-MM-DD")
		}
		attachment.TakenOn = &takenOn
	}

	uploaded, err := mrc.medicalRecordService.UploadAttachment(ctx, userID, role, attachment, file)
	if err != nil {
		return nil, err
	}

	response := toAttachmentResponse(uploaded)
	return &response, nil
}
