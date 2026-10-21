package record

type ListAttachmentsRequest struct {
	PatientID uint64 `uri:"id" form:"-"`
	Limit     int    `uri:"-" form:"limit"`
	Offset    int    `uri:"-" form:"offset"`
}

type ListAttachmentsResponse struct {
	Attachments []AttachmentResponse `json:"attachments"`
}

type GetAttachmentRequest struct {
	ID uint64 `uri:"id"`
}
