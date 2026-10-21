package admin

type RefundPaymentRequest struct {
	ID   uint64 `json:"-" uri:"id" swaggerignore:"true"`
	Note string `json:"note" example:"Doctor unavailable, refunded as goodwill"`
}
