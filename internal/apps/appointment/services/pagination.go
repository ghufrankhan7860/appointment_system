package services

import "appointment_system/internal/apps/appointment/constant"

func clampPagination(limit int, offset int) (int, int) {
	if limit <= 0 {
		limit = constant.DefaultListLimit
	}
	if limit > constant.MaxListLimit {
		limit = constant.MaxListLimit
	}
	if offset < 0 {
		offset = 0
	}

	return limit, offset
}
