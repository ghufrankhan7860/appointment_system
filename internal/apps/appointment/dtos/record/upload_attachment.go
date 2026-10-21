package record

// UploadAttachmentRequest carries the form fields of a multipart upload. ID is
// the patient for a patient-side upload and the record for a doctor's upload.
// The file is the form field "file".
type UploadAttachmentRequest struct {
	ID      uint64 `uri:"id" form:"-"`
	Kind    string `uri:"-" form:"kind" binding:"required"`
	Title   string `uri:"-" form:"title" binding:"required"`
	TakenOn string `uri:"-" form:"takenOn"`
}
