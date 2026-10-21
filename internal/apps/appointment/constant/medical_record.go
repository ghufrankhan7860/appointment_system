package constant

// Limits for medical records and attachments.
const (
	// MaxRecordTextLength is the longest free-text field on a medical record.
	MaxRecordTextLength = 5000
	// MaxPrescriptionItems is the most medicines on one prescription.
	MaxPrescriptionItems = 30
	// MaxAttachmentBytes is the largest file that can be uploaded (10 MB).
	MaxAttachmentBytes = 10 << 20
	// MaxAttachmentTitleLength is the longest attachment title.
	MaxAttachmentTitleLength = 200
	// ContentSniffBytes is how much of an upload is read to detect its real type.
	ContentSniffBytes = 512
	// MultipartOverheadBytes is the room an upload request gets above the file
	// limit for its form fields and multipart framing.
	MultipartOverheadBytes = 1 << 20
	// AttachmentFormField is the multipart field that carries an uploaded file.
	AttachmentFormField = "file"

	// Medicine field lengths match the prescription_items columns.
	MaxMedicineNameLength  = 200
	MaxMedicineFieldLength = 50
	MaxMedicineFormLength  = 30
	MaxMedicineDays        = 3650
)

// Plausible vitals. A value outside these is a typing mistake, not a reading.
const (
	MinPulsePerMinute     = 20
	MaxPulsePerMinute     = 300
	MinTemperatureCelsius = 25.0
	MaxTemperatureCelsius = 45.0
	MinWeightKg           = 0.5
	MaxWeightKg           = 500.0
	MinSpO2Percent        = 50
	MaxSpO2Percent        = 100
	MaxBloodPressureChars = 20
)

// AttachmentContentTypes are the file types an attachment can be. The type is
// detected from the file's content, never taken from the client.
var AttachmentContentTypes = []string{"application/pdf", "image/jpeg", "image/png"}

// AttachmentExtensions name the downloaded file for each allowed type.
var AttachmentExtensions = map[string]string{"application/pdf": ".pdf", "image/jpeg": ".jpg", "image/png": ".png"}
