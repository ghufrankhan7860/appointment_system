package handler

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/controller"
	"appointment_system/internal/apps/appointment/dtos/patient"
	"appointment_system/middleware"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type PatientHandler struct {
	patientController *controller.PatientController
	tokenVerifier     middleware.TokenVerifier
}

func NewPatientHandler(
	patientController *controller.PatientController,
	tokenVerifier middleware.TokenVerifier,
) (*PatientHandler, error) {
	if patientController == nil {
		return nil, apperrors.ErrNilPatientController
	}
	if tokenVerifier == nil {
		return nil, apperrors.ErrNilTokenVerifier
	}
	return &PatientHandler{
		patientController: patientController,
		tokenVerifier:     tokenVerifier,
	}, nil
}

func (ph *PatientHandler) RegisterRoutes(router *gin.RouterGroup) {
	patients := router.Group("/patients", middleware.Authenticate(ph.tokenVerifier))
	patients.POST("", ph.Create)
	patients.GET("", ph.List)
	patients.GET("/:id", ph.GetByID)
	patients.PUT("/:id", ph.Update)
}

func (ph *PatientHandler) Create(ctx *gin.Context) {
	var createPatientRequest patient.CreatePatientRequest
	if err := ctx.ShouldBindJSON(&createPatientRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid request payload"})
		return
	}

	response, err := ph.patientController.Create(ctx.Request.Context(), &createPatientRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPatientInput),
			errors.Is(err, apperrors.ErrNilPatientRequest):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid patient details"})
		case errors.Is(err, apperrors.ErrUserPatientAlreadyExists):
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "Patient already linked to user"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to create patient"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    response,
		"message": "Patient created successfully",
	})
}

func (ph *PatientHandler) GetByID(ctx *gin.Context) {
	var getPatientRequest patient.GetPatientRequest
	if err := ctx.ShouldBindUri(&getPatientRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid patient id"})
		return
	}

	response, err := ph.patientController.GetByID(ctx.Request.Context(), &getPatientRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPatientInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid patient id"})
		case errors.Is(err, apperrors.ErrPatientNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Patient not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to get patient"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Patient retrieved successfully",
	})
}

func (ph *PatientHandler) List(ctx *gin.Context) {
	var listPatientsRequest patient.ListPatientsRequest
	if err := ctx.ShouldBindQuery(&listPatientsRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid query parameters"})
		return
	}

	response, err := ph.patientController.List(ctx.Request.Context(), &listPatientsRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPatientInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid query parameters"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list patients"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Patients retrieved successfully",
	})
}

func (ph *PatientHandler) Update(ctx *gin.Context) {
	var updatePatientRequest patient.UpdatePatientRequest
	if err := ctx.ShouldBindJSON(&updatePatientRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid request payload"})
		return
	}
	if err := ctx.ShouldBindUri(&updatePatientRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid patient id"})
		return
	}

	response, err := ph.patientController.Update(ctx.Request.Context(), &updatePatientRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPatientInput),
			errors.Is(err, apperrors.ErrNilPatientRequest):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid patient details"})
		case errors.Is(err, apperrors.ErrPatientForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Patient access forbidden"})
		case errors.Is(err, apperrors.ErrPatientNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Patient not found"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to update patient"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Patient updated successfully",
	})
}
