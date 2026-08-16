package auth

type RegisterRequest struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
	MobileNo string `json:"mobileNo" binding:"required"`
	Role     string `json:"role" binding:"required,oneof=patient doctor"`
}

type RegisterResponse struct {
	ID       uint64 `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	MobileNo string `json:"mobileNo"`
	Role     string `json:"role"`
}
