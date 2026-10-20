package handler

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/authz"
	"appointment_system/internal/apps/appointment/controller"
	"appointment_system/internal/apps/appointment/dtos/doctor"
	"appointment_system/middleware"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type DoctorHandler struct {
	doctorController *controller.DoctorController
	tokenVerifier    middleware.TokenVerifier
}

func NewDoctorHandler(
	doctorController *controller.DoctorController,
	tokenVerifier middleware.TokenVerifier,
) (*DoctorHandler, error) {
	if doctorController == nil {
		return nil, apperrors.ErrNilDoctorController
	}
	if tokenVerifier == nil {
		return nil, apperrors.ErrNilTokenVerifier
	}
	return &DoctorHandler{
		doctorController: doctorController,
		tokenVerifier:    tokenVerifier,
	}, nil
}

func (dh *DoctorHandler) RegisterRoutes(router *gin.RouterGroup) {
	authenticate := middleware.Authenticate(dh.tokenVerifier)
	manageProfile := authz.RequirePermission(authz.PermDoctorProfileManage)

	doctors := router.Group("/doctors", authenticate)
	doctors.POST("", manageProfile, dh.Create)
	doctors.GET("/me", manageProfile, dh.GetMe)
	doctors.PUT("/me", manageProfile, dh.Update)
	doctors.GET("/me/availability", manageProfile, dh.GetAvailability)
	doctors.PUT("/me/availability", manageProfile, dh.ReplaceAvailability)
	doctors.POST("/me/overrides", manageProfile, dh.AddOverride)
	doctors.GET("", dh.List)
	doctors.GET("/:id", dh.GetByID)
	doctors.GET("/:id/slots", dh.ListSlots)

	router.GET("/specialities", authenticate, dh.ListSpecialities)
}

// Create godoc
// @Summary Create the caller's doctor profile
// @Description Creates the profile for a doctor account. It starts as pending and is not searchable until an admin verifies it.
// @Tags doctors
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body doctor.CreateDoctorRequest true "Doctor profile"
// @Success 201 {object} dtos.SuccessResponse{data=doctor.DoctorResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 422 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors [post]
func (dh *DoctorHandler) Create(ctx *gin.Context) {
	var createDoctorRequest doctor.CreateDoctorRequest
	if err := ctx.ShouldBindJSON(&createDoctorRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := dh.doctorController.Create(ctx.Request.Context(), &createDoctorRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts can create a doctor profile"})
		case errors.Is(err, apperrors.ErrInvalidDoctorInput),
			errors.Is(err, apperrors.ErrNilDoctorRequest):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid doctor details",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrDoctorAlreadyExists):
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "Doctor profile already exists"})
		case errors.Is(err, apperrors.ErrDoctorRegistrationExists):
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "Registration number already registered with this council"})
		case errors.Is(err, apperrors.ErrSpecialityNotFound):
			ctx.JSON(http.StatusUnprocessableEntity, gin.H{
				"success": false,
				"error":   "Unknown speciality",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to create doctor profile"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    response,
		"message": "Doctor profile created successfully",
	})
}

// GetMe godoc
// @Summary Get the caller's doctor profile
// @Tags doctors
// @Security BearerAuth
// @Produce json
// @Success 200 {object} dtos.SuccessResponse{data=doctor.DoctorResponse}
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me [get]
func (dh *DoctorHandler) GetMe(ctx *gin.Context) {
	response, err := dh.doctorController.GetMe(ctx.Request.Context())
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts have a doctor profile"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor profile not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to get doctor profile"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Doctor profile retrieved successfully",
	})
}

// Update godoc
// @Summary Update the caller's doctor profile
// @Description Replaces the profile. A rejected profile, or a verified one whose registration details change, goes back to pending.
// @Tags doctors
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body doctor.UpdateDoctorRequest true "Doctor profile"
// @Success 200 {object} dtos.SuccessResponse{data=doctor.DoctorResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 422 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me [put]
func (dh *DoctorHandler) Update(ctx *gin.Context) {
	var updateDoctorRequest doctor.UpdateDoctorRequest
	if err := ctx.ShouldBindJSON(&updateDoctorRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := dh.doctorController.Update(ctx.Request.Context(), &updateDoctorRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts can update a doctor profile"})
		case errors.Is(err, apperrors.ErrInvalidDoctorInput),
			errors.Is(err, apperrors.ErrNilDoctorRequest):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid doctor details",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor profile not found"})
		case errors.Is(err, apperrors.ErrDoctorRegistrationExists):
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "Registration number already registered with this council"})
		case errors.Is(err, apperrors.ErrSpecialityNotFound):
			ctx.JSON(http.StatusUnprocessableEntity, gin.H{
				"success": false,
				"error":   "Unknown speciality",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to update doctor profile"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Doctor profile updated successfully",
	})
}

// List godoc
// @Summary Search doctors
// @Description Lists verified doctors who accept patients. Speciality and tag match any of the given values; all other filters must all match.
// @Tags doctors
// @Security BearerAuth
// @Produce json
// @Param q query string false "Free text over name, qualification, bio and tags"
// @Param speciality query []string false "Speciality slug, repeatable" collectionFormat(multi)
// @Param tag query []string false "Exact tag, repeatable" collectionFormat(multi)
// @Param city query string false "City, case-insensitive"
// @Param mode query string false "Consultation mode" Enums(in_person, video, phone)
// @Param feeMax query int false "Maximum consultation fee in paise"
// @Param limit query int false "Page size, default 20, max 100"
// @Param offset query int false "Number of doctors to skip"
// @Success 200 {object} dtos.SuccessResponse{data=doctor.ListDoctorsResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors [get]
func (dh *DoctorHandler) List(ctx *gin.Context) {
	var listDoctorsRequest doctor.ListDoctorsRequest
	if err := ctx.ShouldBindQuery(&listDoctorsRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid query parameters",
			"details": bindErrorDetail(err),
		})
		return
	}
	if listDoctorsRequest.Date != "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "The date filter is not supported yet"})
		return
	}

	response, err := dh.doctorController.List(ctx.Request.Context(), &listDoctorsRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrInvalidDoctorInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid query parameters",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to search doctors"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Doctors retrieved successfully",
	})
}

// GetByID godoc
// @Summary Get a doctor's public profile
// @Description Returns a verified doctor. Doctors in any other status are reported as not found.
// @Tags doctors
// @Security BearerAuth
// @Produce json
// @Param id path int true "Doctor ID"
// @Success 200 {object} dtos.SuccessResponse{data=doctor.DoctorResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/{id} [get]
func (dh *DoctorHandler) GetByID(ctx *gin.Context) {
	var getDoctorRequest doctor.GetDoctorRequest
	if err := ctx.ShouldBindUri(&getDoctorRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid doctor id"})
		return
	}

	response, err := dh.doctorController.GetByID(ctx.Request.Context(), &getDoctorRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrInvalidDoctorInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid doctor id"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to get doctor"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Doctor retrieved successfully",
	})
}

// ListSpecialities godoc
// @Summary List specialities
// @Description The controlled list of speciality slugs a doctor profile and search can use.
// @Tags doctors
// @Security BearerAuth
// @Produce json
// @Success 200 {object} dtos.SuccessResponse{data=doctor.ListSpecialitiesResponse}
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /specialities [get]
func (dh *DoctorHandler) ListSpecialities(ctx *gin.Context) {
	response, err := dh.doctorController.ListSpecialities(ctx.Request.Context())
	if err != nil {
		_ = ctx.Error(err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list specialities"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Specialities retrieved successfully",
	})
}

// GetAvailability godoc
// @Summary Get the caller's availability
// @Description Weekly rules and the overrides from today on. Times are in the clinic timezone.
// @Tags doctors
// @Security BearerAuth
// @Produce json
// @Success 200 {object} dtos.SuccessResponse{data=doctor.GetAvailabilityResponse}
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/availability [get]
func (dh *DoctorHandler) GetAvailability(ctx *gin.Context) {
	response, err := dh.doctorController.GetAvailability(ctx.Request.Context())
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts have availability"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor profile not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to get availability"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Availability retrieved successfully",
	})
}

// ReplaceAvailability godoc
// @Summary Replace the caller's weekly availability
// @Description Replaces every weekly rule. dayOfWeek is 0 (Sunday) to 6 (Saturday); times are HH:MM in the clinic timezone, endTime may be 24:00. Blocks on the same day may not overlap, each must fit one slot, and the mode must be one the doctor offers. An empty list clears the schedule.
// @Tags doctors
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body doctor.ReplaceAvailabilityRequest true "Weekly rules"
// @Success 200 {object} dtos.SuccessResponse{data=doctor.ReplaceAvailabilityResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/availability [put]
func (dh *DoctorHandler) ReplaceAvailability(ctx *gin.Context) {
	var replaceAvailabilityRequest doctor.ReplaceAvailabilityRequest
	if err := ctx.ShouldBindJSON(&replaceAvailabilityRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := dh.doctorController.ReplaceAvailability(ctx.Request.Context(), &replaceAvailabilityRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts have availability"})
		case errors.Is(err, apperrors.ErrInvalidAvailabilityInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid availability",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor profile not found"})
		case errors.Is(err, apperrors.ErrAvailabilityOverlap):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "Availability blocks overlap on the same day",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to update availability"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Availability updated successfully",
	})
}

// AddOverride godoc
// @Summary Add a leave or an extra session
// @Description For one date from today up to a year ahead. Not available without times is a whole day off; not available with times is a partial leave; available with times is an extra session.
// @Tags doctors
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body doctor.CreateOverrideRequest true "Override"
// @Success 201 {object} dtos.SuccessResponse{data=doctor.OverrideResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/overrides [post]
func (dh *DoctorHandler) AddOverride(ctx *gin.Context) {
	var createOverrideRequest doctor.CreateOverrideRequest
	if err := ctx.ShouldBindJSON(&createOverrideRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := dh.doctorController.AddOverride(ctx.Request.Context(), &createOverrideRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts have availability"})
		case errors.Is(err, apperrors.ErrInvalidAvailabilityInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid override",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor profile not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to add override"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    response,
		"message": "Override added successfully",
	})
}

// ListSlots godoc
// @Summary List a doctor's free slots for a day
// @Description Free slots of a verified doctor on one date, from today up to 90 days ahead: weekly rules, minus leave, plus extra sessions. Slots already started today are left out.
// @Tags doctors
// @Security BearerAuth
// @Produce json
// @Param id path int true "Doctor ID"
// @Param date query string true "Date as YYYY-MM-DD"
// @Success 200 {object} dtos.SuccessResponse{data=doctor.ListSlotsResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/{id}/slots [get]
func (dh *DoctorHandler) ListSlots(ctx *gin.Context) {
	var listSlotsRequest doctor.ListSlotsRequest
	if err := ctx.ShouldBindUri(&listSlotsRequest); err != nil || listSlotsRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid doctor id"})
		return
	}
	if err := ctx.ShouldBindQuery(&listSlotsRequest); err != nil || listSlotsRequest.Date == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Date is required as YYYY-MM-DD"})
		return
	}

	response, err := dh.doctorController.ListSlots(ctx.Request.Context(), &listSlotsRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrInvalidAvailabilityInput),
			errors.Is(err, apperrors.ErrInvalidDoctorInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid slot date",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list slots"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Slots retrieved successfully",
	})
}
