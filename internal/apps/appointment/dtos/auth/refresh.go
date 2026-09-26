package auth

type RefreshRequest struct {
	Token string `json:"token" binding:"required"`
}

type RefreshResponse struct {
	JwtToken     string `json:"jwtToken"`
	RefreshToken string `json:"refreshToken"`
}
