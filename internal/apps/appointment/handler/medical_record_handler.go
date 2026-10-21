package handler

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/authz"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/controller"
	"appointment_system/internal/apps/appointment/dtos/record"
	"appointment_system/middleware"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type MedicalRecordHandler struct {
	medicalRecordController *controller.MedicalRecordController
	tokenVerifier           middleware.TokenVerifier
}

func NewMedicalRecordHandler(
	medicalRecordController *controller.MedicalRecordController,
	tokenVerifier middleware.TokenVerifier,
) (*MedicalRecordHandler, error) {
	if medicalRecordController == nil {
		return nil, apperrors.ErrNilMedicalRecordController
	}
	if tokenVerifier == nil {
		return nil, apperrors.ErrNilTokenVerifier
	}
	return &MedicalRecordHandler{
		medicalRecordController: medicalRecordController,
		tokenVerifier:           tokenVerifier,
	}, nil
}

func (mrh *MedicalRecordHandler) RegisterRoutes(router *gin.RouterGroup) {
	authenticate := middleware.Authenticate(mrh.tokenVerifier)
	write := authz.RequirePermission(authz.PermRecordWrite)

	router.POST("/doctors/me/appointments/:id/record", authenticate, write, mrh.CreateDraft)
	router.PUT("/doctors/me/records/:id", authenticate, write, mrh.UpdateDraft)
	router.POST("/doctors/me/records/:id/finalize", authenticate, write, mrh.Finalize)
	router.POST("/doctors/me/records/:id/attachments", authenticate, write, mrh.UploadForRecord)

	router.GET("/records/:id", authenticate, mrh.GetByID)
	router.GET("/patients/:id/records", authenticate, mrh.ListForPatient)
	router.GET("/patients/:id/attachments", authenticate, mrh.ListAttachments)
	router.POST("/patients/:id/attachments", authenticate, mrh.UploadForPatient)
	router.GET("/attachments/:id/content", authenticate, mrh.Download)
}

// CreateDraft godoc
// @Summary Open the medical record of an appointment
// @Description Opens a draft record for one of the caller's appointments once it has started and is not cancelled. Asking again returns the existing record.
// @Tags medical records
// @Security BearerAuth
// @Produce json
// @Param id path int true "Appointment ID"
// @Success 201 {object} dtos.SuccessResponse{data=record.MedicalRecordResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/appointments/{id}/record [post]
func (mrh *MedicalRecordHandler) CreateDraft(ctx *gin.Context) {
	var createRecordRequest record.CreateRecordRequest
	if err := ctx.ShouldBindUri(&createRecordRequest); err != nil || createRecordRequest.AppointmentID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		return
	}

	response, err := mrh.medicalRecordController.CreateDraft(ctx.Request.Context(), &createRecordRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts can write medical records"})
		case errors.Is(err, apperrors.ErrAppointmentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Appointment not found"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor profile not found"})
		case errors.Is(err, apperrors.ErrAppointmentNotModifiable):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "A record cannot be opened for this appointment",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to open medical record"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    response,
		"message": "Medical record opened successfully",
	})
}

// UpdateDraft godoc
// @Summary Edit a draft medical record
// @Description Replaces the draft's notes, vitals and prescription. A finalized record cannot change.
// @Tags medical records
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Medical record ID"
// @Param request body record.UpdateRecordRequest true "Record contents"
// @Success 200 {object} dtos.SuccessResponse{data=record.MedicalRecordResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/records/{id} [put]
func (mrh *MedicalRecordHandler) UpdateDraft(ctx *gin.Context) {
	var updateRecordRequest record.UpdateRecordRequest
	if err := ctx.ShouldBindJSON(&updateRecordRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid medical record",
			"details": bindErrorDetail(err),
		})
		return
	}
	if err := ctx.ShouldBindUri(&updateRecordRequest); err != nil || updateRecordRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid medical record id"})
		return
	}

	response, err := mrh.medicalRecordController.UpdateDraft(ctx.Request.Context(), &updateRecordRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidMedicalRecordInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid medical record",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts can write medical records"})
		case errors.Is(err, apperrors.ErrMedicalRecordNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Medical record not found"})
		case errors.Is(err, apperrors.ErrMedicalRecordFinalized):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "This medical record can no longer change",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to update medical record"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Medical record updated successfully",
	})
}

// Finalize godoc
// @Summary Finalize a medical record
// @Description Locks the draft for good and makes it readable by the patient side with full access. It needs a diagnosis.
// @Tags medical records
// @Security BearerAuth
// @Produce json
// @Param id path int true "Medical record ID"
// @Success 200 {object} dtos.SuccessResponse{data=record.MedicalRecordResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/records/{id}/finalize [post]
func (mrh *MedicalRecordHandler) Finalize(ctx *gin.Context) {
	var finalizeRecordRequest record.FinalizeRecordRequest
	if err := ctx.ShouldBindUri(&finalizeRecordRequest); err != nil || finalizeRecordRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid medical record id"})
		return
	}

	response, err := mrh.medicalRecordController.Finalize(ctx.Request.Context(), &finalizeRecordRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidMedicalRecordInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "This medical record is not ready to finalize",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts can write medical records"})
		case errors.Is(err, apperrors.ErrMedicalRecordNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Medical record not found"})
		case errors.Is(err, apperrors.ErrMedicalRecordFinalized):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "This medical record is already finalized",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to finalize medical record"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Medical record finalized successfully",
	})
}

// GetByID godoc
// @Summary Get a medical record
// @Description Returns a record with its attachments. The record's doctor sees it in any status; the patient side needs full access and sees finalized records only. Anyone else gets 404. Every read is written to the access log.
// @Tags medical records
// @Security BearerAuth
// @Produce json
// @Param id path int true "Medical record ID"
// @Success 200 {object} dtos.SuccessResponse{data=record.MedicalRecordResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /records/{id} [get]
func (mrh *MedicalRecordHandler) GetByID(ctx *gin.Context) {
	var getRecordRequest record.GetRecordRequest
	if err := ctx.ShouldBindUri(&getRecordRequest); err != nil || getRecordRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid medical record id"})
		return
	}

	response, err := mrh.medicalRecordController.GetByID(ctx.Request.Context(), &getRecordRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrMedicalRecordNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Medical record not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to get medical record"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Medical record retrieved successfully",
	})
}

// ListForPatient godoc
// @Summary List a patient's medical records
// @Description Lists the patient's finalized records, newest first. Needs full access to the patient; other access levels get 404. Limit defaults to 20 and is capped at 100.
// @Tags medical records
// @Security BearerAuth
// @Produce json
// @Param id path int true "Patient ID"
// @Param limit query int false "Page size"
// @Param offset query int false "Number of records to skip"
// @Success 200 {object} dtos.SuccessResponse{data=record.ListRecordsResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /patients/{id}/records [get]
func (mrh *MedicalRecordHandler) ListForPatient(ctx *gin.Context) {
	var listRecordsRequest record.ListRecordsRequest
	if err := ctx.ShouldBindUri(&listRecordsRequest); err != nil || listRecordsRequest.PatientID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid patient id"})
		return
	}
	if err := ctx.ShouldBindQuery(&listRecordsRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid query parameters",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := mrh.medicalRecordController.ListForPatient(ctx.Request.Context(), &listRecordsRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrPatientNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Patient not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list medical records"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Medical records retrieved successfully",
	})
}

// UploadForPatient godoc
// @Summary Upload a report for a patient
// @Description Uploads a lab report, scan or other file for a patient the caller has full access to. Only PDF, JPEG and PNG are kept, detected from the file's content, up to 10 MB.
// @Tags medical records
// @Security BearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param id path int true "Patient ID"
// @Param file formData file true "PDF, JPEG or PNG, at most 10 MB"
// @Param kind formData string true "Kind of file" Enums(lab_report, imaging, prescription_scan, referral, other)
// @Param title formData string true "Short title"
// @Param takenOn formData string false "Date the report was taken, as YYYY-MM-DD"
// @Success 201 {object} dtos.SuccessResponse{data=record.AttachmentResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 413 {object} dtos.ErrorResponse
// @Failure 415 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /patients/{id}/attachments [post]
func (mrh *MedicalRecordHandler) UploadForPatient(ctx *gin.Context) {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, constant.MaxAttachmentBytes+constant.MultipartOverheadBytes)

	var uploadAttachmentRequest record.UploadAttachmentRequest
	if err := ctx.ShouldBind(&uploadAttachmentRequest); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			ctx.JSON(http.StatusRequestEntityTooLarge, gin.H{"success": false, "error": "File is too large"})
			return
		}
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid upload",
			"details": bindErrorDetail(err),
		})
		return
	}
	if err := ctx.ShouldBindUri(&uploadAttachmentRequest); err != nil || uploadAttachmentRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid patient id"})
		return
	}
	fileHeader, err := ctx.FormFile(constant.AttachmentFormField)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid upload",
			"details": "file is required",
		})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		_ = ctx.Error(err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to read upload"})
		return
	}
	defer file.Close()

	response, err := mrh.medicalRecordController.UploadForPatient(ctx.Request.Context(), &uploadAttachmentRequest, file)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidMedicalRecordInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid upload",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrPatientNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Patient not found"})
		case errors.Is(err, apperrors.ErrAttachmentTooLarge):
			ctx.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"success": false,
				"error":   "File is too large",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrAttachmentType):
			ctx.JSON(http.StatusUnsupportedMediaType, gin.H{
				"success": false,
				"error":   "File type is not allowed",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to upload file"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    response,
		"message": "File uploaded successfully",
	})
}

// UploadForRecord godoc
// @Summary Add a file to a draft medical record
// @Description The record's doctor adds a lab report, scan or other file to their draft. Only PDF, JPEG and PNG are kept, detected from the file's content, up to 10 MB.
// @Tags medical records
// @Security BearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param id path int true "Medical record ID"
// @Param file formData file true "PDF, JPEG or PNG, at most 10 MB"
// @Param kind formData string true "Kind of file" Enums(lab_report, imaging, prescription_scan, referral, other)
// @Param title formData string true "Short title"
// @Param takenOn formData string false "Date the report was taken, as YYYY-MM-DD"
// @Success 201 {object} dtos.SuccessResponse{data=record.AttachmentResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 413 {object} dtos.ErrorResponse
// @Failure 415 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/records/{id}/attachments [post]
func (mrh *MedicalRecordHandler) UploadForRecord(ctx *gin.Context) {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, constant.MaxAttachmentBytes+constant.MultipartOverheadBytes)

	var uploadAttachmentRequest record.UploadAttachmentRequest
	if err := ctx.ShouldBind(&uploadAttachmentRequest); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			ctx.JSON(http.StatusRequestEntityTooLarge, gin.H{"success": false, "error": "File is too large"})
			return
		}
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid upload",
			"details": bindErrorDetail(err),
		})
		return
	}
	if err := ctx.ShouldBindUri(&uploadAttachmentRequest); err != nil || uploadAttachmentRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid medical record id"})
		return
	}
	fileHeader, err := ctx.FormFile(constant.AttachmentFormField)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid upload",
			"details": "file is required",
		})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		_ = ctx.Error(err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to read upload"})
		return
	}
	defer file.Close()

	response, err := mrh.medicalRecordController.UploadForRecord(ctx.Request.Context(), &uploadAttachmentRequest, file)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidMedicalRecordInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid upload",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts can write medical records"})
		case errors.Is(err, apperrors.ErrMedicalRecordNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Medical record not found"})
		case errors.Is(err, apperrors.ErrMedicalRecordFinalized):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "This medical record can no longer change",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrAttachmentTooLarge):
			ctx.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"success": false,
				"error":   "File is too large",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrAttachmentType):
			ctx.JSON(http.StatusUnsupportedMediaType, gin.H{
				"success": false,
				"error":   "File type is not allowed",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to upload file"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    response,
		"message": "File uploaded successfully",
	})
}

// ListAttachments godoc
// @Summary List a patient's uploaded files
// @Description Lists the patient's attachments, newest first. Needs full access to the patient; other access levels get 404. Limit defaults to 20 and is capped at 100.
// @Tags medical records
// @Security BearerAuth
// @Produce json
// @Param id path int true "Patient ID"
// @Param limit query int false "Page size"
// @Param offset query int false "Number of files to skip"
// @Success 200 {object} dtos.SuccessResponse{data=record.ListAttachmentsResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /patients/{id}/attachments [get]
func (mrh *MedicalRecordHandler) ListAttachments(ctx *gin.Context) {
	var listAttachmentsRequest record.ListAttachmentsRequest
	if err := ctx.ShouldBindUri(&listAttachmentsRequest); err != nil || listAttachmentsRequest.PatientID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid patient id"})
		return
	}
	if err := ctx.ShouldBindQuery(&listAttachmentsRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid query parameters",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := mrh.medicalRecordController.ListAttachments(ctx.Request.Context(), &listAttachmentsRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrPatientNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Patient not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list files"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Files retrieved successfully",
	})
}

// Download godoc
// @Summary Download an uploaded file
// @Description Streams the file to the patient side with full access, or to the doctor of the record it belongs to. Anyone else gets 404. Every download is written to the access log.
// @Tags medical records
// @Security BearerAuth
// @Produce application/pdf
// @Produce image/jpeg
// @Produce image/png
// @Param id path int true "Attachment ID"
// @Success 200 {file} file
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /attachments/{id}/content [get]
func (mrh *MedicalRecordHandler) Download(ctx *gin.Context) {
	var getAttachmentRequest record.GetAttachmentRequest
	if err := ctx.ShouldBindUri(&getAttachmentRequest); err != nil || getAttachmentRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid attachment id"})
		return
	}

	attachment, file, err := mrh.medicalRecordController.OpenAttachment(ctx.Request.Context(), &getAttachmentRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrAttachmentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "File not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to download file"})
		}
		return
	}
	defer file.Close()

	ctx.DataFromReader(http.StatusOK, attachment.SizeBytes, attachment.ContentType, file, map[string]string{
		"Content-Disposition":    fmt.Sprintf(`attachment; filename="attachment-%d%s"`, attachment.ID, constant.AttachmentExtensions[attachment.ContentType]),
		"X-Content-Type-Options": "nosniff",
	})
}
