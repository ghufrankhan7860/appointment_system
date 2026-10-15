package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/controller"
	dto "appointment_system/internal/apps/appointment/dtos/patient"
	"appointment_system/middleware"
)

type PatientHandler struct {
	patientController *controller.PatientController
	verifyToken       func(token string) (uint64, error)
}

func NewPatientHandler(patientController *controller.PatientController, verifyToken func(token string) (uint64, error)) (*PatientHandler, error) {
	if patientController == nil {
		return nil, apperrors.ErrNilPatientController
	}
	if verifyToken == nil {
		return nil, apperrors.ErrNilTokenGenerator
	}
	return &PatientHandler{patientController: patientController, verifyToken: verifyToken}, nil
}

func (ph *PatientHandler) RegisterRoutes(router *gin.RouterGroup) {
	patients := router.Group("/patients", middleware.RequireAuth(ph.verifyToken))
	patients.POST("", ph.Create)
	patients.GET("", ph.List)
	patients.GET("/:id", ph.Get)
	patients.PUT("/:id", ph.Update)
}

func (ph *PatientHandler) Create(ctx *gin.Context) {
	userID, _ := middleware.UserID(ctx)
	var request dto.CreateRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid request payload"})
		return
	}

	response, err := ph.patientController.Create(ctx.Request.Context(), userID, &request)
	if err != nil {
		writePatientError(ctx, err, "Failed to create patient")
		return
	}
	ctx.JSON(http.StatusCreated, gin.H{"success": true, "data": response, "message": "Patient created successfully"})
}

func (ph *PatientHandler) List(ctx *gin.Context) {
	userID, _ := middleware.UserID(ctx)
	limit, err := queryInt(ctx, "limit")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid limit"})
		return
	}
	offset, err := queryInt(ctx, "offset")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid offset"})
		return
	}

	response, err := ph.patientController.List(ctx.Request.Context(), userID, limit, offset)
	if err != nil {
		writePatientError(ctx, err, "Failed to list patients")
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": response, "message": "Patients retrieved successfully"})
}

func (ph *PatientHandler) Get(ctx *gin.Context) {
	userID, _ := middleware.UserID(ctx)
	patientID, ok := patientIDParam(ctx)
	if !ok {
		return
	}

	response, err := ph.patientController.Get(ctx.Request.Context(), userID, patientID)
	if err != nil {
		writePatientError(ctx, err, "Failed to get patient")
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": response, "message": "Patient retrieved successfully"})
}

func (ph *PatientHandler) Update(ctx *gin.Context) {
	userID, _ := middleware.UserID(ctx)
	patientID, ok := patientIDParam(ctx)
	if !ok {
		return
	}
	var request dto.UpdateRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid request payload"})
		return
	}

	response, err := ph.patientController.Update(ctx.Request.Context(), userID, patientID, &request)
	if err != nil {
		writePatientError(ctx, err, "Failed to update patient")
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": response, "message": "Patient updated successfully"})
}

// patientIDParam parses :id; on failure it writes the 400 response and returns false.
func patientIDParam(ctx *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil || id == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid patient id"})
		return 0, false
	}
	return id, true
}

// queryInt returns 0 (service default) when the parameter is absent.
func queryInt(ctx *gin.Context, name string) (int, error) {
	value := ctx.Query(name)
	if value == "" {
		return 0, nil
	}
	return strconv.Atoi(value)
}

func writePatientError(ctx *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, apperrors.ErrInvalidPatientInput), errors.Is(err, apperrors.ErrNilPatientRequest):
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid patient data"})
	case errors.Is(err, apperrors.ErrPatientForbidden):
		ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Patient access forbidden"})
	case errors.Is(err, apperrors.ErrPatientNotFound):
		ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Patient not found"})
	case errors.Is(err, apperrors.ErrUserPatientAlreadyExists):
		ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "Patient already linked to user"})
	default:
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": fallback})
	}
}
