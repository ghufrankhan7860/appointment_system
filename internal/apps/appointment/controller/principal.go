package controller

import (
	"appointment_system/internal/appcontext"
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"context"
	"strconv"
)

func actingUserID(ctx context.Context) (uint64, error) {
	applicationContext, ok := appcontext.From(ctx)
	if !ok {
		return 0, apperrors.ErrPrincipalNotFound
	}

	userID, err := strconv.ParseUint(applicationContext.PrincipalID, 10, 64)
	if err != nil || userID == 0 {
		return 0, apperrors.ErrPrincipalNotFound
	}

	return userID, nil
}

func actingRole(ctx context.Context) (models.Role, error) {
	applicationContext, ok := appcontext.From(ctx)
	if !ok || applicationContext.PrincipalRole == "" {
		return "", apperrors.ErrPrincipalNotFound
	}

	return models.Role(applicationContext.PrincipalRole), nil
}
