package admin

type ListDoctorsRequest struct {
	Status string `form:"status"`
	Limit  int    `form:"limit"`
	Offset int    `form:"offset"`
}

type ListDoctorsResponse struct {
	Doctors []GetDoctorResponse `json:"doctors"`
}
