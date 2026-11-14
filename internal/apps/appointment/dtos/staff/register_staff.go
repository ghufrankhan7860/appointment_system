package staff

// RegisterStaffRequest signs up a clinic staff account with the invite code a
// doctor gave. The response is auth.RegisterResponse with role staff.
type RegisterStaffRequest struct {
	Name       string `json:"name" binding:"required" example:"Ravi Kumar"`
	Email      string `json:"email" binding:"required,email" example:"ravi@example.com"`
	Password   string `json:"password" binding:"required,min=8,max=72"`
	MobileNo   string `json:"mobileNo" binding:"required" example:"9876543210"`
	InviteCode string `json:"inviteCode" binding:"required" example:"K7Q2-M9XD"`
}
