package doctor

type ListDoctorsRequest struct {
	Query        string   `form:"q"`
	Specialities []string `form:"speciality"`
	Tags         []string `form:"tag"`
	City         string   `form:"city"`
	Mode         string   `form:"mode"`
	Date         string   `form:"date"`
	FeeMax       int      `form:"feeMax"`
	Limit        int      `form:"limit"`
	Offset       int      `form:"offset"`
}

type ListDoctorsResponse struct {
	Doctors []DoctorResponse `json:"doctors"`
}
