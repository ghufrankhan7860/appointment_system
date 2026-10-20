package doctor

import "time"

type ClinicRequest struct {
	Name    string `json:"name" example:"Verma Clinic"`
	Address string `json:"address" example:"12 Park Street"`
	City    string `json:"city" example:"New Delhi"`
	Pincode string `json:"pincode" example:"110001"`
}

type ClinicResponse struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	City    string `json:"city"`
	Pincode string `json:"pincode"`
}

type SpecialityResponse struct {
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	IsPrimary bool   `json:"isPrimary"`
}

// DoctorResponse is the doctor profile returned by every doctor endpoint.
type DoctorResponse struct {
	ID                  uint64               `json:"id"`
	FullName            string               `json:"fullName"`
	RegistrationNumber  string               `json:"registrationNumber"`
	RegistrationCouncil string               `json:"registrationCouncil"`
	RegistrationYear    int                  `json:"registrationYear"`
	Qualification       string               `json:"qualification"`
	YearsOfExperience   int                  `json:"yearsOfExperience"`
	Bio                 string               `json:"bio"`
	ConsultationFee     int                  `json:"consultationFee"`
	SlotDurationMin     int                  `json:"slotDurationMin"`
	ConsultationModes   []string             `json:"consultationModes"`
	Specialities        []SpecialityResponse `json:"specialities"`
	Tags                []string             `json:"tags"`
	Clinic              ClinicResponse       `json:"clinic"`
	VerificationStatus  string               `json:"verificationStatus" example:"verified"`
	VerifiedAt          *time.Time           `json:"verifiedAt"`
	IsAcceptingPatients bool                 `json:"isAcceptingPatients"`
}
