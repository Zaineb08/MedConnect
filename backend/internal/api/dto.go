// Package api defines request/response DTOs for the MedConnect Oriental API.
package api

import (
	"time"

	"medconnect-oriental/backend/internal/models"

	"github.com/google/uuid"
)

// ──────────────────────────────────────────────────────────────────────
// Referral DTOs
// ──────────────────────────────────────────────────────────────────────

// CreateReferralRequest is the payload from a Level 2 doctor creating a referral.
type CreateReferralRequest struct {
	// Patient info (will be encrypted before storage)
	PatientCIN   string `json:"patient_cin" binding:"required"`
	PatientName  string `json:"patient_name" binding:"required"`
	PatientDOB   string `json:"patient_dob" binding:"required"`
	PatientPhone string `json:"patient_phone" binding:"required"`
	PatientConsent *bool `json:"patient_consent"`

	// Referral info
	DepartmentID string              `json:"department_id" binding:"required,uuid"`
	Symptoms     string              `json:"symptoms" binding:"required,min=10"`
	Urgency      models.UrgencyLevel `json:"urgency" binding:"required,oneof=LOW MEDIUM HIGH CRITICAL"`

	// Optional: AI suggestion that was accepted
	AISuggestedDept *string `json:"ai_suggested_dept,omitempty"`
}

// SuggestDepartmentRequest is the payload for the AI triage endpoint.
type SuggestDepartmentRequest struct {
	Symptoms   string `json:"symptoms" binding:"required,min=10"`
	PatientDOB string `json:"patient_dob" binding:"required"`
}

// SuggestDepartmentResponse is the AI's recommendation.
type SuggestDepartmentResponse struct {
	SuggestedDepartment string  `json:"suggested_department"`
	Urgency             string  `json:"urgency"`
	Confidence          float64 `json:"confidence"`
	Reasoning           string  `json:"reasoning"`
}

// ScheduleReferralRequest is the payload for scheduling an appointment.
type ScheduleReferralRequest struct {
	AppointmentDate string `json:"appointment_date" binding:"required"` // ISO 8601: "2026-04-15T10:00:00Z"
}

// RedirectReferralRequest is the payload for redirecting to another department.
type RedirectReferralRequest struct {
	NewDepartmentID string `json:"new_department_id" binding:"required,uuid"`
	Reason          string `json:"reason" binding:"required,min=5"`
}

// DenyReferralRequest is the payload for denying a referral (reason mandatory).
type DenyReferralRequest struct {
	Reason string `json:"reason" binding:"required,min=10"`
}

// ──────────────────────────────────────────────────────────────────────
// Admin DTOs
// ──────────────────────────────────────────────────────────────────────

type CreateUserRequest struct {
	Username     string      `json:"username" binding:"required"`
	Password     string      `json:"password" binding:"required,min=8"`
	Role         models.Role `json:"role" binding:"required,oneof=SUPER_ADMIN ANALYST CHU_DOC LEVEL_2_DOC"`
	FacilityName string      `json:"facility_name" binding:"required"`
	DepartmentID *string     `json:"department_id,omitempty"` // For CHU_DOC
}

type CreateDepartmentRequest struct {
	Name           string `json:"name" binding:"required"`
	PhoneExtension string `json:"phone_extension"`
	WorkHours      string `json:"work_hours"`
	WorkDays       string `json:"work_days"`
}

type UpdateDepartmentRequest struct {
	Name           *string `json:"name"`
	PhoneExtension *string `json:"phone_extension"`
	WorkHours      *string `json:"work_hours"`
	WorkDays       *string `json:"work_days"`
	IsAccepting    *bool   `json:"is_accepting"`
}

// ──────────────────────────────────────────────────────────────────────
// Response DTOs
// ──────────────────────────────────────────────────────────────────────

// ReferralResponse is the decrypted view of a referral for API consumers.
type ReferralResponse struct {
	ID              uuid.UUID             `json:"id"`
	PatientCIN      string                `json:"patient_cin"`
	PatientName     string                `json:"patient_name"`
	PatientDOB      string                `json:"patient_dob"`
	PatientPhone    string                `json:"patient_phone"`
	PatientConsent  bool                  `json:"patient_consent"`
	ConsentTimestamp time.Time            `json:"consent_timestamp"`
	CreatorUsername string                `json:"creator_username"`
	CreatorFacility string                `json:"creator_facility"`
	Department      string                `json:"department"`
	DepartmentID    uuid.UUID             `json:"department_id"`
	Status          models.ReferralStatus `json:"status"`
	Urgency         models.UrgencyLevel   `json:"urgency"`
	Symptoms        string                `json:"symptoms"`
	AISuggestedDept *string               `json:"ai_suggested_dept,omitempty"`
	AISummary       *string               `json:"ai_summary,omitempty"`
	AppointmentDate *time.Time            `json:"appointment_date,omitempty"`
	RejectionReason *string               `json:"rejection_reason,omitempty"`
	Attachments     []AttachmentResponse  `json:"attachments,omitempty"`
	CreatedAt       time.Time             `json:"created_at"`
	UpdatedAt       time.Time             `json:"updated_at"`
}

type AttachmentResponse struct {
	ID       uuid.UUID `json:"id"`
	FileName string    `json:"file_name"`
	FileType string    `json:"file_type"`
	FileSize int64     `json:"file_size"`
}

// QueueItem is a lighter referral view for the CHU doctor's triage queue.
type QueueItem struct {
	ID              uuid.UUID             `json:"id"`
	PatientName     string                `json:"patient_name"`
	PatientDOB      string                `json:"patient_dob"`
	Urgency         models.UrgencyLevel   `json:"urgency"`
	AISummary       *string               `json:"ai_summary,omitempty"`
	Status          models.ReferralStatus `json:"status"`
	CreatorFacility string                `json:"creator_facility"`
	CreatedAt       time.Time             `json:"created_at"`
	HasAttachments  bool                  `json:"has_attachments"`
}

// NotificationResponse is a notification item for Level 2 doctors.
type NotificationResponse struct {
	ID         uuid.UUID `json:"id"`
	ReferralID uuid.UUID `json:"referral_id"`
	Message    string    `json:"message"`
	IsRead     bool      `json:"is_read"`
	CreatedAt  time.Time `json:"created_at"`
}

// PaginationMeta contains pagination metadata for list endpoints.
type PaginationMeta struct {
	Limit   int   `json:"limit"`
	Offset  int   `json:"offset"`
	Total   int64 `json:"total"`
	HasNext bool  `json:"has_next"`
	HasPrev bool  `json:"has_prev"`
}
