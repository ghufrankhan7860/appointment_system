package record

type ListRecordsRequest struct {
	PatientID uint64 `uri:"id" form:"-"`
	Limit     int    `uri:"-" form:"limit"`
	Offset    int    `uri:"-" form:"offset"`
}

type ListRecordsResponse struct {
	Records []MedicalRecordResponse `json:"records"`
}
