package admin

import "appointment_system/internal/apps/appointment/dtos/payment"

type ListRefundsRequest struct {
	Status string `form:"status"`
	Limit  int    `form:"limit"`
	Offset int    `form:"offset"`
}

type ListRefundsResponse struct {
	Refunds []payment.RefundResponse `json:"refunds"`
}
