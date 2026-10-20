package doctor

type CreateDoctorRequest struct {
	FullName            string        `json:"fullName" binding:"required" example:"Dr. Asha Verma"`
	RegistrationNumber  string        `json:"registrationNumber" binding:"required" example:"DMC/R/12345"`
	RegistrationCouncil string        `json:"registrationCouncil" binding:"required" example:"Delhi Medical Council"`
	RegistrationYear    int           `json:"registrationYear" example:"2012"`
	Qualification       string        `json:"qualification" binding:"required" example:"MBBS, MD (Medicine)"`
	YearsOfExperience   int           `json:"yearsOfExperience" binding:"min=0" example:"11"`
	Bio                 string        `json:"bio" example:"Physician focused on diabetes and thyroid care."`
	ConsultationFee     int           `json:"consultationFee" binding:"min=0" example:"60000"`
	SlotDurationMin     int           `json:"slotDurationMin" example:"15"`
	ConsultationModes   []string      `json:"consultationModes" binding:"required,min=1" example:"in_person,video"`
	Specialities        []string      `json:"specialities" binding:"required,min=1" example:"general-medicine,diabetology"`
	Tags                []string      `json:"tags" example:"diabetes,thyroid,hindi,english"`
	Clinic              ClinicRequest `json:"clinic"`
}
