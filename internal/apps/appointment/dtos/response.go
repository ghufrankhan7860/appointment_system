package dtos

// SuccessResponse and ErrorResponse describe the JSON envelope written by the
// handlers. They exist only so the Swagger annotations can reference it.
type SuccessResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message"`
}

type ErrorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error" example:"Invalid patient details"`
	Details string `json:"details,omitempty" example:"dateOfBirth cannot be in the future"`
}
