package admin

type CompleteRefundRequest struct {
	ID        uint64 `json:"-" uri:"id" swaggerignore:"true"`
	Reference string `json:"reference" binding:"required" example:"412345678901"`
}
