package patient

type ListPatientsRequest struct {
	Limit  int `form:"limit"`
	Offset int `form:"offset"`
}

type ListPatientsResponse struct {
	Patients []GetPatientResponse `json:"patients"`
}
