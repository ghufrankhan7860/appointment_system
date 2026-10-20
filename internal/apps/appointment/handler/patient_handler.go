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

// Create godoc
// @Summary Create a patient
// @Description Creates a patient and links it to the caller with full access.
// @Tags patients
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body patient.CreatePatientRequest true "Patient details"
// @Success 201 {object} dtos.SuccessResponse{data=patient.CreatePatientResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /patients [post]
func (ph *PatientHandler) Create(ctx *gin.Context) {
	var createPatientRequest patient.CreatePatientRequest
	if err := ctx.ShouldBindJSON(&createPatientRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := ph.patientController.Create(ctx.Request.Context(), &createPatientRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPatientInput),
			errors.Is(err, apperrors.ErrNilPatientRequest):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid patient details",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrUserPatientAlreadyExists):
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "Patient already linked to user"})
		default:
			_ = ctx.Error(err)
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

// GetByID godoc
// @Summary Get a patient
// @Description Returns a patient the caller has active access to. Other patients are reported as not found.
// @Tags patients
// @Security BearerAuth
// @Produce json
// @Param id path int true "Patient ID"
// @Success 200 {object} dtos.SuccessResponse{data=patient.GetPatientResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /patients/{id} [get]
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
			_ = ctx.Error(err)
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

// List godoc
// @Summary List patients
// @Description Lists the patients the caller has active access to. Limit defaults to 20 and is capped at 100.
// @Tags patients
// @Security BearerAuth
// @Produce json
// @Param limit query int false "Page size"
// @Param offset query int false "Number of patients to skip"
// @Success 200 {object} dtos.SuccessResponse{data=patient.ListPatientsResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /patients [get]
func (ph *PatientHandler) List(ctx *gin.Context) {
	var listPatientsRequest patient.ListPatientsRequest
	if err := ctx.ShouldBindQuery(&listPatientsRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid query parameters",
			"details": bindErrorDetail(err),
		})
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
			_ = ctx.Error(err)
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

// Update godoc
// @Summary Update a patient
// @Description Updates a patient. Requires full access to the patient.
// @Tags patients
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Patient ID"
// @Param request body patient.UpdatePatientRequest true "Patient details"
// @Success 200 {object} dtos.SuccessResponse{data=patient.UpdatePatientResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /patients/{id} [put]
func (ph *PatientHandler) Update(ctx *gin.Context) {
	var updatePatientRequest patient.UpdatePatientRequest
	if err := ctx.ShouldBindJSON(&updatePatientRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
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
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid patient details",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrPatientForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Patient access forbidden"})
		case errors.Is(err, apperrors.ErrPatientNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Patient not found"})
		default:
			_ = ctx.Error(err)
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
