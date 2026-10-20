package admin

type CreateAdminRequest struct {
	Name     string `json:"name" binding:"required" example:"Ops Admin"`
	Email    string `json:"email" binding:"required,email" example:"ops@example.com"`
	Password string `json:"password" binding:"required,min=12,max=72"`
	MobileNo string `json:"mobileNo" binding:"required" example:"9876543210"`
}

type CreateAdminResponse struct {
	ID       uint64 `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	MobileNo string `json:"mobileNo"`
	Role     string `json:"role" example:"admin"`
}
