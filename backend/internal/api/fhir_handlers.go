package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"medconnect-oriental/backend/internal/models"
)

// Minimal FHIR R4 resources used for SIH interoperability pull APIs.
type fhirMeta struct {
	LastUpdated string `json:"lastUpdated,omitempty"`
}

type fhirReference struct {
	Reference string `json:"reference,omitempty"`
	Display   string `json:"display,omitempty"`
}

type fhirIdentifier struct {
	Use    string `json:"use,omitempty"`
	System string `json:"system,omitempty"`
	Value  string `json:"value,omitempty"`
}

type fhirCoding struct {
	System  string `json:"system,omitempty"`
	Code    string `json:"code,omitempty"`
	Display string `json:"display,omitempty"`
}

type fhirCodeableConcept struct {
	Coding []fhirCoding `json:"coding,omitempty"`
	Text   string       `json:"text,omitempty"`
}

type fhirPatientResource struct {
	ResourceType string           `json:"resourceType"`
	ID           string           `json:"id"`
	Meta         fhirMeta         `json:"meta,omitempty"`
	Identifier   []fhirIdentifier `json:"identifier,omitempty"`
	Name         []struct {
		Text string `json:"text,omitempty"`
	} `json:"name,omitempty"`
	Gender    string `json:"gender,omitempty"`
	BirthDate string `json:"birthDate,omitempty"`
}

type fhirServiceRequestResource struct {
	ResourceType string              `json:"resourceType"`
	ID           string              `json:"id"`
	Meta         fhirMeta            `json:"meta,omitempty"`
	Status       string              `json:"status"`
	Intent       string              `json:"intent"`
	Priority     string              `json:"priority,omitempty"`
	Code         fhirCodeableConcept `json:"code,omitempty"`
	Subject      fhirReference       `json:"subject"`
	Requester    fhirReference       `json:"requester,omitempty"`
	AuthoredOn   string              `json:"authoredOn,omitempty"`
	ReasonCode   []fhirCodeableConcept `json:"reasonCode,omitempty"`
}

type fhirEncounterResource struct {
	ResourceType string        `json:"resourceType"`
	ID           string        `json:"id"`
	Meta         fhirMeta      `json:"meta,omitempty"`
	Status       string        `json:"status"`
	Class        fhirCoding    `json:"class,omitempty"`
	Subject      fhirReference `json:"subject"`
	Period       struct {
		Start string `json:"start,omitempty"`
		End   string `json:"end,omitempty"`
	} `json:"period,omitempty"`
	ServiceProvider fhirReference `json:"serviceProvider,omitempty"`
}

type fhirBundleResource struct {
	ResourceType string `json:"resourceType"`
	Type         string `json:"type"`
	Entry        []struct {
		Resource interface{} `json:"resource"`
	} `json:"entry"`
}

func mapReferralStatusToFHIRServiceRequestStatus(status models.ReferralStatus) string {
	switch status {
	case models.StatusScheduled:
		return "active"
	case models.StatusDenied, models.StatusCanceled:
		return "revoked"
	default:
		return "active"
	}
}

func mapReferralStatusToFHIREncounterStatus(status models.ReferralStatus) string {
	switch status {
	case models.StatusScheduled:
		return "planned"
	case models.StatusCanceled:
		return "cancelled"
	case models.StatusDenied:
		return "cancelled"
	default:
		return "in-progress"
	}
}

func mapUrgencyToFHIRPriority(urgency models.UrgencyLevel) string {
	switch urgency {
	case models.UrgencyCritical, models.UrgencyHigh:
		return "urgent"
	case models.UrgencyLow:
		return "routine"
	default:
		return "asap"
	}
}

func (h *HandlerContext) mapPatientToFHIRResource(patient models.Patient, name, cin string) fhirPatientResource {
	resource := fhirPatientResource{
		ResourceType: "Patient",
		ID:           patient.ID.String(),
		Meta:         fhirMeta{LastUpdated: patient.UpdatedAt.UTC().Format(time.RFC3339)},
		Identifier: []fhirIdentifier{
			{
				Use:    "official",
				System: "urn:medconnect:patient:cin",
				Value:  cin,
			},
		},
		BirthDate: patient.DateOfBirth.Format("2006-01-02"),
		Gender:    "unknown",
	}

	resource.Name = append(resource.Name, struct {
		Text string `json:"text,omitempty"`
	}{Text: name})

	return resource
}

func (h *HandlerContext) mapReferralToFHIRServiceRequestResource(ref models.Referral, symptoms string) fhirServiceRequestResource {
	resource := fhirServiceRequestResource{
		ResourceType: "ServiceRequest",
		ID:           ref.ID.String(),
		Meta:         fhirMeta{LastUpdated: ref.UpdatedAt.UTC().Format(time.RFC3339)},
		Status:       mapReferralStatusToFHIRServiceRequestStatus(ref.Status),
		Intent:       "order",
		Priority:     mapUrgencyToFHIRPriority(ref.Urgency),
		Code: fhirCodeableConcept{
			Text: "Referral to " + ref.Department.Name,
		},
		Subject: fhirReference{
			Reference: "Patient/" + ref.PatientID.String(),
		},
		Requester: fhirReference{
			Reference: "Practitioner/" + ref.CreatorID.String(),
			Display:   ref.Creator.Username,
		},
		AuthoredOn: ref.CreatedAt.UTC().Format(time.RFC3339),
		ReasonCode: []fhirCodeableConcept{
			{Text: symptoms},
		},
	}

	return resource
}

func (h *HandlerContext) mapReferralToFHIREncounterResource(ref models.Referral) fhirEncounterResource {
	resource := fhirEncounterResource{
		ResourceType: "Encounter",
		ID:           ref.ID.String(),
		Meta:         fhirMeta{LastUpdated: ref.UpdatedAt.UTC().Format(time.RFC3339)},
		Status:       mapReferralStatusToFHIREncounterStatus(ref.Status),
		Class: fhirCoding{
			System:  "http://terminology.hl7.org/CodeSystem/v3-ActCode",
			Code:    "AMB",
			Display: "ambulatory",
		},
		Subject: fhirReference{
			Reference: "Patient/" + ref.PatientID.String(),
		},
		ServiceProvider: fhirReference{
			Reference: "Organization/" + ref.CurrentDeptID.String(),
			Display:   ref.Department.Name,
		},
	}

	resource.Period.Start = ref.CreatedAt.UTC().Format(time.RFC3339)
	if ref.AppointmentDate != nil {
		resource.Period.End = ref.AppointmentDate.UTC().Format(time.RFC3339)
	}

	return resource
}

// GET /api/fhir/R4/Patient/:id
func (h *HandlerContext) GetFHIRPatient(c *gin.Context) {
	patientID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid patient ID"})
		return
	}

	var patient models.Patient
	if err := h.DB.First(&patient, "id = ?", patientID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Patient not found"})
		return
	}

	decryptedName := h.decryptPatientField(patient.ID, "fullname", patient.FullName, "[Decryption Error]")
	decryptedCIN := h.decryptPatientField(patient.ID, "cin", patient.CIN, "[Decryption Error]")

	c.JSON(http.StatusOK, h.mapPatientToFHIRResource(patient, decryptedName, decryptedCIN))
}

// GET /api/fhir/R4/ServiceRequest/:id
func (h *HandlerContext) GetFHIRServiceRequest(c *gin.Context) {
	referralID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid referral ID"})
		return
	}

	var referral models.Referral
	if err := h.DB.Preload("Creator").Preload("Department").First(&referral, "id = ?", referralID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Referral not found"})
		return
	}

	symptoms := h.decryptReferralField(referral.ID, referral.PatientID, "symptoms", referral.Symptoms, "[Decryption Error]")

	c.JSON(http.StatusOK, h.mapReferralToFHIRServiceRequestResource(referral, symptoms))
}

// GET /api/fhir/R4/Encounter/:id
func (h *HandlerContext) GetFHIREncounter(c *gin.Context) {
	referralID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid referral ID"})
		return
	}

	var referral models.Referral
	if err := h.DB.Preload("Department").First(&referral, "id = ?", referralID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Referral not found"})
		return
	}

	c.JSON(http.StatusOK, h.mapReferralToFHIREncounterResource(referral))
}

// GET /api/fhir/R4/referrals/:id
func (h *HandlerContext) GetFHIRReferralBundle(c *gin.Context) {
	referralID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid referral ID"})
		return
	}

	var referral models.Referral
	if err := h.DB.Preload("Creator").Preload("Department").First(&referral, "id = ?", referralID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Referral not found"})
		return
	}

	symptoms := h.decryptReferralField(referral.ID, referral.PatientID, "symptoms", referral.Symptoms, "[Decryption Error]")
	serviceReq := h.mapReferralToFHIRServiceRequestResource(referral, symptoms)
	encounter := h.mapReferralToFHIREncounterResource(referral)

	bundle := fhirBundleResource{
		ResourceType: "Bundle",
		Type:         "collection",
	}
	bundle.Entry = append(bundle.Entry,
		struct {
			Resource interface{} `json:"resource"`
		}{Resource: serviceReq},
		struct {
			Resource interface{} `json:"resource"`
		}{Resource: encounter},
	)

	c.JSON(http.StatusOK, bundle)
}
