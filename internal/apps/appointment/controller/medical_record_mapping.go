package controller

import (
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/dtos/record"
	"appointment_system/internal/apps/appointment/models"
)

func toMedicalRecordResponse(medicalRecord *models.MedicalRecord, attachments []*models.MedicalAttachment) record.MedicalRecordResponse {
	medicines := make([]record.Medicine, 0, len(medicalRecord.Medicines))
	for _, medicine := range medicalRecord.Medicines {
		medicines = append(medicines, record.Medicine{
			MedicineName: medicine.MedicineName,
			Strength:     medicine.Strength,
			Form:         medicine.Form,
			Dosage:       medicine.Dosage,
			Frequency:    medicine.Frequency,
			DurationDays: medicine.DurationDays,
			Timing:       string(medicine.Timing),
			Instructions: medicine.Instructions,
		})
	}

	response := record.MedicalRecordResponse{
		ID:                medicalRecord.ID,
		AppointmentID:     medicalRecord.AppointmentID,
		PatientID:         medicalRecord.PatientID,
		DoctorID:          medicalRecord.DoctorID,
		ChiefComplaint:    medicalRecord.ChiefComplaint,
		Symptoms:          medicalRecord.Symptoms,
		Diagnosis:         medicalRecord.Diagnosis,
		ExaminationNotes:  medicalRecord.ExaminationNotes,
		Advice:            medicalRecord.Advice,
		PrescriptionNotes: medicalRecord.PrescriptionNotes,
		Vitals: record.Vitals{
			BloodPressure:         medicalRecord.Vitals.BloodPressure,
			PulsePerMinute:        medicalRecord.Vitals.PulsePerMinute,
			TemperatureFahrenheit: medicalRecord.Vitals.TemperatureFahrenheit,
			WeightKg:              medicalRecord.Vitals.WeightKg,
			SpO2Percent:           medicalRecord.Vitals.SpO2Percent,
		},
		Medicines:   medicines,
		Status:      string(medicalRecord.Status),
		FinalizedAt: medicalRecord.FinalizedAt,
		CreatedAt:   medicalRecord.CreatedAt,
		UpdatedAt:   medicalRecord.UpdatedAt,
	}
	if medicalRecord.FollowUpOn != nil {
		response.FollowUpOn = medicalRecord.FollowUpOn.Format(constant.DateLayout)
	}
	for _, attachment := range attachments {
		response.Attachments = append(response.Attachments, toAttachmentResponse(attachment))
	}

	return response
}

func toAttachmentResponse(attachment *models.MedicalAttachment) record.AttachmentResponse {
	response := record.AttachmentResponse{
		ID:              attachment.ID,
		PatientID:       attachment.PatientID,
		MedicalRecordID: attachment.MedicalRecordID,
		Kind:            string(attachment.Kind),
		Title:           attachment.Title,
		ContentType:     attachment.ContentType,
		SizeBytes:       attachment.SizeBytes,
		CreatedAt:       attachment.CreatedAt,
	}
	if attachment.TakenOn != nil {
		response.TakenOn = attachment.TakenOn.Format(constant.DateLayout)
	}

	return response
}
