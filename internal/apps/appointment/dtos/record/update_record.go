package record

// UpdateRecordRequest replaces a draft's notes, vitals and prescription.
type UpdateRecordRequest struct {
	ID                uint64     `json:"-" uri:"id" swaggerignore:"true"`
	ChiefComplaint    string     `json:"chiefComplaint" example:"Fever for three days"`
	Symptoms          string     `json:"symptoms" example:"Fever, body ache"`
	Diagnosis         string     `json:"diagnosis" example:"Viral fever"`
	ExaminationNotes  string     `json:"examinationNotes"`
	Advice            string     `json:"advice" example:"Rest and fluids"`
	PrescriptionNotes string     `json:"prescriptionNotes"`
	FollowUpOn        string     `json:"followUpOn" example:"2026-11-02"`
	Vitals            Vitals     `json:"vitals"`
	Medicines         []Medicine `json:"medicines"`
}
