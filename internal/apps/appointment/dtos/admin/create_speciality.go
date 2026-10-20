package admin

type CreateSpecialityRequest struct {
	Slug string `json:"slug" binding:"required" example:"nephrology"`
	Name string `json:"name" binding:"required" example:"Nephrology"`
}

type CreateSpecialityResponse struct {
	ID   uint64 `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}
