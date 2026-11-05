package services

import (
	"fmt"
	"strings"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
)

func normalizeMedicalRecord(record *models.MedicalRecord) {
	record.ChiefComplaint = strings.TrimSpace(record.ChiefComplaint)
	record.Symptoms = strings.TrimSpace(record.Symptoms)
	record.Diagnosis = strings.TrimSpace(record.Diagnosis)
	record.ExaminationNotes = strings.TrimSpace(record.ExaminationNotes)
	record.Advice = strings.TrimSpace(record.Advice)
	record.PrescriptionNotes = strings.TrimSpace(record.PrescriptionNotes)
	record.Vitals.BloodPressure = strings.TrimSpace(record.Vitals.BloodPressure)

	for index := range record.Medicines {
		medicine := &record.Medicines[index]
		medicine.MedicineName = strings.TrimSpace(medicine.MedicineName)
		medicine.Strength = strings.TrimSpace(medicine.Strength)
		medicine.Form = strings.TrimSpace(medicine.Form)
		medicine.Dosage = strings.TrimSpace(medicine.Dosage)
		medicine.Frequency = strings.TrimSpace(medicine.Frequency)
		medicine.Instructions = strings.TrimSpace(medicine.Instructions)
		if medicine.Timing == "" {
			medicine.Timing = models.MedicineTimingAny
		}
	}
}

func validateMedicalRecord(record *models.MedicalRecord) error {
	texts := []struct {
		field string
		value string
	}{
		{field: "chiefComplaint", value: record.ChiefComplaint},
		{field: "symptoms", value: record.Symptoms},
		{field: "diagnosis", value: record.Diagnosis},
		{field: "examinationNotes", value: record.ExaminationNotes},
		{field: "advice", value: record.Advice},
		{field: "prescriptionNotes", value: record.PrescriptionNotes},
	}
	for _, text := range texts {
		if len(text.value) > constant.MaxRecordTextLength {
			return apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, text.field, fmt.Sprintf("must be at most %d characters", constant.MaxRecordTextLength))
		}
	}

	if err := validateVitals(record.Vitals); err != nil {
		return err
	}

	if len(record.Medicines) > constant.MaxPrescriptionItems {
		return apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, "medicines", fmt.Sprintf("must have at most %d items", constant.MaxPrescriptionItems))
	}
	for _, medicine := range record.Medicines {
		if err := validateMedicine(medicine); err != nil {
			return err
		}
	}

	return nil
}

func validateVitals(vitals models.Vitals) error {
	if len(vitals.BloodPressure) > constant.MaxBloodPressureChars {
		return apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, "vitals.bloodPressure", fmt.Sprintf("must be at most %d characters", constant.MaxBloodPressureChars))
	}
	if vitals.PulsePerMinute != nil && (*vitals.PulsePerMinute < constant.MinPulsePerMinute || *vitals.PulsePerMinute > constant.MaxPulsePerMinute) {
		return apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, "vitals.pulsePerMinute", fmt.Sprintf("must be between %d and %d", constant.MinPulsePerMinute, constant.MaxPulsePerMinute))
	}
	if vitals.TemperatureFahrenheit != nil && (*vitals.TemperatureFahrenheit < constant.MinTemperatureFahrenheit || *vitals.TemperatureFahrenheit > constant.MaxTemperatureFahrenheit) {
		return apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, "vitals.temperatureFahrenheit", fmt.Sprintf("must be between %g and %g", constant.MinTemperatureFahrenheit, constant.MaxTemperatureFahrenheit))
	}
	if vitals.WeightKg != nil && (*vitals.WeightKg < constant.MinWeightKg || *vitals.WeightKg > constant.MaxWeightKg) {
		return apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, "vitals.weightKg", fmt.Sprintf("must be between %g and %g", constant.MinWeightKg, constant.MaxWeightKg))
	}
	if vitals.SpO2Percent != nil && (*vitals.SpO2Percent < constant.MinSpO2Percent || *vitals.SpO2Percent > constant.MaxSpO2Percent) {
		return apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, "vitals.spo2Percent", fmt.Sprintf("must be between %d and %d", constant.MinSpO2Percent, constant.MaxSpO2Percent))
	}

	return nil
}

func validateMedicine(medicine models.PrescriptionItem) error {
	if medicine.MedicineName == "" {
		return apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, "medicines.medicineName", "is required")
	}
	if medicine.Dosage == "" {
		return apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, "medicines.dosage", "is required")
	}
	if len(medicine.MedicineName) > constant.MaxMedicineNameLength {
		return apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, "medicines.medicineName", fmt.Sprintf("must be at most %d characters", constant.MaxMedicineNameLength))
	}
	shortFields := []struct {
		field string
		value string
	}{
		{field: "medicines.strength", value: medicine.Strength},
		{field: "medicines.dosage", value: medicine.Dosage},
		{field: "medicines.frequency", value: medicine.Frequency},
	}
	for _, short := range shortFields {
		if len(short.value) > constant.MaxMedicineFieldLength {
			return apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, short.field, fmt.Sprintf("must be at most %d characters", constant.MaxMedicineFieldLength))
		}
	}
	if len(medicine.Form) > constant.MaxMedicineFormLength {
		return apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, "medicines.form", fmt.Sprintf("must be at most %d characters", constant.MaxMedicineFormLength))
	}
	if len(medicine.Instructions) > constant.MaxRecordTextLength {
		return apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, "medicines.instructions", fmt.Sprintf("must be at most %d characters", constant.MaxRecordTextLength))
	}
	if medicine.DurationDays < 0 || medicine.DurationDays > constant.MaxMedicineDays {
		return apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, "medicines.durationDays", fmt.Sprintf("must be between 0 and %d", constant.MaxMedicineDays))
	}
	if !validMedicineTiming(medicine.Timing) {
		return apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, "medicines.timing", "must be one of before_food, after_food, with_food, any")
	}

	return nil
}

func validateAttachment(attachment *models.MedicalAttachment) error {
	if !validAttachmentKind(attachment.Kind) {
		return apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, "kind", "must be one of lab_report, imaging, prescription_scan, referral, other")
	}
	if attachment.Title == "" {
		return apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, "title", "is required")
	}
	if len(attachment.Title) > constant.MaxAttachmentTitleLength {
		return apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, "title", fmt.Sprintf("must be at most %d characters", constant.MaxAttachmentTitleLength))
	}

	return nil
}

func validMedicineTiming(timing models.MedicineTiming) bool {
	switch timing {
	case models.MedicineTimingBeforeFood, models.MedicineTimingAfterFood,
		models.MedicineTimingWithFood, models.MedicineTimingAny:
		return true
	default:
		return false
	}
}

func validAttachmentKind(kind models.AttachmentKind) bool {
	switch kind {
	case models.AttachmentKindLabReport, models.AttachmentKindImaging,
		models.AttachmentKindPrescriptionScan, models.AttachmentKindReferral, models.AttachmentKindOther:
		return true
	default:
		return false
	}
}
