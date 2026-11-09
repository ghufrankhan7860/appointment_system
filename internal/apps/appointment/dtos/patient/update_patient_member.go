package patient

type UpdatePatientMemberRequest struct {
	ID          uint64 `json:"-" uri:"id" swaggerignore:"true"`
	UserID      uint64 `json:"-" uri:"userId" swaggerignore:"true"`
	AccessLevel string `json:"accessLevel" binding:"required" enums:"full,appointments_only,read_only" example:"read_only"`
}
