package auth

type LoginRequest struct {
	Email    *string `json:"email"`
	MobileNo *string `json:"mobileNo"`
	Password string  `json:"password" binding:"required,min=6,max=72"`
}

// ClientHeaders is what login and refresh read about the client from request
// headers. The X-Device-* and X-App-Version headers are sent by the app and are
// optional; User-Agent and Accept-Language come from the browser.
type ClientHeaders struct {
	UserAgent      string `header:"User-Agent"`
	AcceptLanguage string `header:"Accept-Language"`
	DeviceID       string `header:"X-Device-Id" binding:"omitempty,max=64"`
	DeviceName     string `header:"X-Device-Name" binding:"omitempty,max=100"`
	DevicePlatform string `header:"X-Device-Platform" binding:"omitempty,oneof=web android ios"`
	AppVersion     string `header:"X-App-Version" binding:"omitempty,max=30"`
	Screen         string `header:"X-Device-Screen" binding:"omitempty,max=20"`
	Timezone       string `header:"X-Device-Timezone" binding:"omitempty,max=64"`
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
