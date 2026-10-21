package admin

import "time"

type ListAccessLogRequest struct {
	ID     uint64 `uri:"id" form:"-"`
	Limit  int    `uri:"-" form:"limit"`
	Offset int    `uri:"-" form:"offset"`
}

// RecordAccessResponse is one access to a patient's clinical data. It names
// who and how, never what the record says.
type RecordAccessResponse struct {
	ID               uint64    `json:"id"`
	PatientID        uint64    `json:"patientId"`
	MedicalRecordID  *uint64   `json:"medicalRecordId,omitempty"`
	AttachmentID     *uint64   `json:"attachmentId,omitempty"`
	AccessedByUserID uint64    `json:"accessedByUserId"`
	Action           string    `json:"action" enums:"create,update,finalize,view,list,upload,download" example:"view"`
	CorrelationID    string    `json:"correlationId"`
	CreatedAt        time.Time `json:"createdAt"`
}

type ListAccessLogResponse struct {
	Entries []RecordAccessResponse `json:"entries"`
}
