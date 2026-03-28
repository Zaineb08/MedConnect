// Package models defines the GORM database models for MedConnect Oriental.
// Sensitive fields (CIN, FullName, Symptoms) are stored as encrypted hex strings
// using the internal/crypto AES-256-GCM module — encryption/decryption happens
// at the service layer, NOT via GORM hooks, for explicit control.
package models

import (
	"time"

	"github.com/google/uuid"
)

// ──────────────────────────────────────────────────────────────────────
// Role constants
// ──────────────────────────────────────────────────────────────────────

type Role string

const (
	RoleSuperAdmin Role = "SUPER_ADMIN"
	RoleAnalyst    Role = "ANALYST"
	RoleCHUDoc     Role = "CHU_DOC"
	RoleLevel2Doc  Role = "LEVEL_2_DOC"
)

// ──────────────────────────────────────────────────────────────────────
// Referral Status constants
// ──────────────────────────────────────────────────────────────────────

type ReferralStatus string

const (
	StatusPending    ReferralStatus = "PENDING"
	StatusScheduled  ReferralStatus = "SCHEDULED"
	StatusRedirected ReferralStatus = "REDIRECTED"
	StatusDenied     ReferralStatus = "DENIED"
	StatusCanceled   ReferralStatus = "CANCELED"
)

// ──────────────────────────────────────────────────────────────────────
// Urgency Level constants
// ──────────────────────────────────────────────────────────────────────

type UrgencyLevel string

const (
	UrgencyLow      UrgencyLevel = "LOW"
	UrgencyMedium   UrgencyLevel = "MEDIUM"
	UrgencyHigh     UrgencyLevel = "HIGH"
	UrgencyCritical UrgencyLevel = "CRITICAL"
)

// ──────────────────────────────────────────────────────────────────────
// User — Doctors, Admins, Analysts
// ──────────────────────────────────────────────────────────────────────

type User struct {
	ID           uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Username     string     `gorm:"uniqueIndex;size:100;not null" json:"username"`
	PasswordHash string     `gorm:"size:255;not null" json:"-"` // bcrypt hash, never serialized
	Role         Role       `gorm:"size:20;not null;index" json:"role"`
	DeptID       *uuid.UUID `gorm:"type:uuid;index" json:"dept_id,omitempty"` // NULL for SUPER_ADMIN/ANALYST
	FacilityName string     `gorm:"size:200;not null" json:"facility_name"`
	IsActive     bool       `gorm:"default:true;not null" json:"is_active"`
	CreatedAt    time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"autoUpdateTime" json:"updated_at"`

	// Relations
	Department *Department `gorm:"foreignKey:DeptID" json:"department,omitempty"`
}

// ──────────────────────────────────────────────────────────────────────
// Department — CHU specialist departments
// ──────────────────────────────────────────────────────────────────────

type Department struct {
	ID             uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Name           string    `gorm:"uniqueIndex;size:200;not null" json:"name"`
	PhoneExtension string    `gorm:"size:20" json:"phone_extension"`
	WorkHours      string    `gorm:"size:100" json:"work_hours"` // e.g. "08:00-16:00"
	WorkDays       string    `gorm:"size:100" json:"work_days"`  // e.g. "Lun-Ven"
	IsAccepting    bool      `gorm:"default:true;not null" json:"is_accepting"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// ──────────────────────────────────────────────────────────────────────
// Patient — PII fields (CIN, FullName) stored AES-256-GCM encrypted
// ──────────────────────────────────────────────────────────────────────

type Patient struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	CIN         string    `gorm:"size:500;not null" json:"cin"`       // ENCRYPTED
	FullName    string    `gorm:"size:500;not null" json:"full_name"` // ENCRYPTED
	DateOfBirth time.Time `gorm:"type:date;not null;default:'2000-01-01'" json:"date_of_birth"`
	PhoneNumber string    `gorm:"size:20" json:"phone_number"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// Relations
	Referrals []Referral `gorm:"foreignKey:PatientID" json:"referrals,omitempty"`
}

// ──────────────────────────────────────────────────────────────────────
// Referral — The core workflow entity
// ──────────────────────────────────────────────────────────────────────

type Referral struct {
	ID              uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	PatientID       uuid.UUID      `gorm:"type:uuid;not null;index" json:"patient_id"`
	CreatorID       uuid.UUID      `gorm:"type:uuid;not null;index;index:idx_creator_status" json:"creator_id"`
	CurrentDeptID   uuid.UUID      `gorm:"type:uuid;not null;index;index:idx_dept_status,priority:1;index:idx_dept_urgency,priority:1" json:"current_dept_id"`
	PatientConsent  bool           `gorm:"not null;default:false" json:"patient_consent"`
	ConsentTimestamp time.Time     `gorm:"not null;default:CURRENT_TIMESTAMP" json:"consent_timestamp"`
	Status          ReferralStatus `gorm:"size:20;not null;default:'PENDING';index;index:idx_dept_status,priority:2;index:idx_creator_status,priority:2" json:"status"`
	Urgency         UrgencyLevel   `gorm:"size:20;not null;default:'MEDIUM';index:idx_dept_urgency,priority:2" json:"urgency"`
	Symptoms        string         `gorm:"type:text;not null" json:"symptoms"`          // ENCRYPTED
	AISuggestedDept *string        `gorm:"size:200" json:"ai_suggested_dept,omitempty"` // AI recommendation
	AISummary       *string        `gorm:"type:text" json:"ai_summary,omitempty"`       // AI-generated TL;DR
	AppointmentDate *time.Time     `json:"appointment_date,omitempty"`
	RejectionReason *string        `gorm:"type:text" json:"rejection_reason,omitempty"`
	CreatedAt       time.Time      `gorm:"autoCreateTime;index" json:"created_at"`
	UpdatedAt       time.Time      `gorm:"autoUpdateTime" json:"updated_at"`

	// Relations
	Patient     Patient      `gorm:"foreignKey:PatientID" json:"patient,omitempty"`
	Creator     User         `gorm:"foreignKey:CreatorID" json:"creator,omitempty"`
	Department  Department   `gorm:"foreignKey:CurrentDeptID" json:"department,omitempty"`
	Attachments []Attachment `gorm:"foreignKey:ReferralID" json:"attachments,omitempty"`
}

// ──────────────────────────────────────────────────────────────────────
// Attachment — Secure file references (stored on local volume)
// ──────────────────────────────────────────────────────────────────────

type Attachment struct {
	ID         uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	ReferralID uuid.UUID `gorm:"type:uuid;not null;index" json:"referral_id"`
	FilePath   string    `gorm:"size:500;not null" json:"-"` // Never exposed via API
	FileName   string    `gorm:"size:200;not null" json:"file_name"`
	FileType   string    `gorm:"size:50;not null" json:"file_type"` // e.g. "image/jpeg", "application/pdf"
	FileSize   int64     `gorm:"not null" json:"file_size"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// ──────────────────────────────────────────────────────────────────────
// AuditLog — Immutable compliance ledger (Moroccan Law 09-08)
// ──────────────────────────────────────────────────────────────────────

type AuditLog struct {
	ID        uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID    *uuid.UUID `gorm:"type:uuid;index" json:"user_id"`      // NULL for unauthenticated
	Username  string     `gorm:"size:100" json:"username"`            // Denormalized for fast reads
	Action    string     `gorm:"size:200;not null" json:"action"`     // e.g. "POST /api/referrals"
	TargetID  string     `gorm:"size:100" json:"target_id,omitempty"` // Resource ID acted upon
	IPAddress string     `gorm:"size:45;not null" json:"ip_address"`  // IPv4 or IPv6
	UserAgent string     `gorm:"size:500" json:"user_agent"`
	Status    int        `gorm:"not null" json:"status"` // HTTP status code
	Timestamp time.Time  `gorm:"autoCreateTime;index" json:"timestamp"`
}

// TableName overrides GORM's default table name for AuditLog.
func (AuditLog) TableName() string {
	return "audit_logs"
}
