package auth

type LoginRequest struct {
	Email    *string `json:"email"`
	MobileNo *string `json:"mobileNo"`
	Password string  `json:"password" binding:"required,min=6,max=72"`
}

type LoginResponse struct {
	ID           uint64 `json:"id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	MobileNo     string `json:"mobileNo"`
	Role         string `json:"role"`
	JwtToken     string `json:"jwtToken"`
	RefreshToken string `json:"refreshToken"`
}
