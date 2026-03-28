// Package ai provides a client for the local Ollama instance running
// Llama-3-8B-Instruct. All AI inference is local — no cloud LLM calls.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Service handles all AI inference via the local Ollama instance.
type Service struct {
	ollamaURL  string
	model      string
	httpClient *http.Client
}

// NewService creates a new AI service connected to the local Ollama.
func NewService(ollamaURL, model string) *Service {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   100,
		MaxConnsPerHost:       200,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 25 * time.Second,
	}

	return &Service{
		ollamaURL: strings.TrimRight(ollamaURL, "/"),
		model:     model,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   45 * time.Second,
		},
	}
}

// ──────────────────────────────────────────────────────────────────────
// Ollama API types
// ──────────────────────────────────────────────────────────────────────

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// ──────────────────────────────────────────────────────────────────────
// 1. Smart Triage Routing
// ──────────────────────────────────────────────────────────────────────

// DepartmentSuggestion holds the AI's triage recommendation and urgency.
type DepartmentSuggestion struct {
	Department string  `json:"department"`
	Urgency    string  `json:"urgency"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning"`
}

// SuggestDepartment analyzes symptoms and patient age to recommend
// the most appropriate CHU department for referral.
func (s *Service) SuggestDepartment(symptoms string, patientAge int, departments []string) (*DepartmentSuggestion, error) {
	deptList := strings.Join(departments, ", ")

	prompt := fmt.Sprintf(`You are a clinical triage assistant at CHU Mohammed VI in Oujda, Morocco.
Your task is to analyze patient symptoms and recommend both the MOST appropriate department for referral AND the clinical urgency.

AVAILABLE DEPARTMENTS: %s
URGENCY LEVELS: LOW, MEDIUM, HIGH, CRITICAL

PATIENT INFORMATION:
- Age: %d years old
- Symptoms: %s

IMPORTANT RULES:
1. Consider the patient's age when routing (e.g., children under 16 should go to Pediatric departments when available).
2. If symptoms suggest multiple departments, choose the most primary one.
3. Be specific — do not suggest "General Medicine" if a specialist matches.
4. Assess urgency based on symptom severity (e.g. chest pain -> CRITICAL, mild rash -> LOW).

RESPOND IN EXACTLY THIS JSON FORMAT (no markdown, no extra text):
{"department": "<exact department name from the list>", "urgency": "<LOW|MEDIUM|HIGH|CRITICAL>", "confidence": <0.0-1.0>, "reasoning": "<1-2 sentence clinical reasoning IN FRENCH>"}`, deptList, patientAge, symptoms)

	response, err := s.generate(prompt)
	if err != nil {
		return nil, fmt.Errorf("ai: triage suggestion failed: %w", err)
	}

	var suggestion DepartmentSuggestion
	// Try to parse JSON from the response (handle potential extra text)
	jsonStr := extractJSON(response)
	if err := json.Unmarshal([]byte(jsonStr), &suggestion); err != nil {
		// Fallback: return raw response as reasoning
		return &DepartmentSuggestion{
			Department: "Unknown",
			Urgency:    "MEDIUM",
			Confidence: 0.5,
			Reasoning:  response,
		}, nil
	}

	return &suggestion, nil
}

// ──────────────────────────────────────────────────────────────────────
// 2. WhatsApp Message Generation (Darija + French)
// ──────────────────────────────────────────────────────────────────────

// GenerateWhatsAppMessage creates a culturally appropriate appointment
// notification in Moroccan Darija and French, with document checklists.
func (s *Service) GenerateWhatsAppMessage(patientName, symptoms, deptName string, appointmentDate time.Time) (string, error) {
	dateStr := appointmentDate.Format("02/01/2006 à 15:04")

	prompt := fmt.Sprintf(`You are a medical communication assistant at CHU Mohammed VI in Oujda, Morocco.
Generate a WhatsApp appointment notification message for a patient.

PATIENT: %s
DEPARTMENT: %s
APPOINTMENT DATE: %s
SYMPTOMS/REASON: %s

RULES:
1. Write the message in TWO languages:
   - First in Moroccan Darija (written in Arabic script)
   - Then a French translation below
2. Be warm, respectful, and professional.
3. Include a DOCUMENT CHECKLIST specific to the symptoms/department:
   - e.g., for cardiology: ECG results, recent blood test (NFS, lipid profile)
   - e.g., for neurology: recent MRI/CT scan, previous neurological reports
   - e.g., for surgery: pre-op blood tests, X-rays of affected area
4. Include practical information: bring CIN/ID card, arrive 30 minutes early.
5. Include the CHU address: CHU Mohammed VI, Boulevard Mohammed VI, Oujda.
6. Keep the message under 500 words total.
7. Do NOT use markdown formatting — this is a plain WhatsApp message.
8. Use emojis moderately for readability (✅, 📋, 🏥, 📅).

Generate ONLY the message, no additional commentary.`, patientName, deptName, dateStr, symptoms)

	response, err := s.generate(prompt)
	if err != nil {
		return "", fmt.Errorf("ai: WhatsApp message generation failed: %w", err)
	}

	return strings.TrimSpace(response), nil
}

// GenerateSurvivalChecklist creates a bilingual (Darija + French) practical
// checklist sent to patients before departing for CHU.
func (s *Service) GenerateSurvivalChecklist(symptoms string, dept string) (string, error) {
	prompt := fmt.Sprintf(`Tu es assistant de préparation patient au CHU Mohammed VI d'Oujda.
Ta mission: générer une "Checklist de Survie" personnalisée avant départ vers le CHU.

CONTEXTE:
- Département cible: %s
- Symptômes: %s

RÈGLES STRICTES:
1) Répondre dans cet ordre:
   - Partie 1: Darija marocaine (écriture arabe)
   - Partie 2: Français
2) Chaque partie doit contenir une checklist claire avec cases visuelles (✅ ou ☐) couvrant:
   - Documents administratifs à apporter
   - Examens / bilans médicaux à apporter selon symptômes et service
   - Préparation avant départ (jeûne si nécessaire, médicaments, accompagnant, heure d'arrivée)
3) Le ton doit être concret, rassurant, orienté action.
4) Pas de markdown, pas de JSON, pas d'introduction inutile.
5) Longueur maximale: 220 mots.

Génère uniquement le message final prêt pour WhatsApp.`, dept, symptoms)

	response, err := s.generate(prompt)
	if err != nil {
		return "", fmt.Errorf("ai: survival checklist generation failed: %w", err)
	}

	return strings.TrimSpace(response), nil
}

// ──────────────────────────────────────────────────────────────────────
// 3. Executive Summarization
// ──────────────────────────────────────────────────────────────────────

// SummarizeSymptoms condenses lengthy referral text into a 3-line
// TL;DR for rapid CHU triage review.
func (s *Service) SummarizeSymptoms(symptoms string) (string, error) {
	prompt := fmt.Sprintf(`You are a clinical summarization assistant. 
Condense the following patient symptoms/referral notes into EXACTLY 3 concise lines:
- Line 1: Primary complaint
- Line 2: Key clinical findings/history
- Line 3: Suggested urgency assessment

SYMPTOMS:
%s

Respond with ONLY the 3 lines, no headers or extra text. Write in French (medical terminology).`, symptoms)

	response, err := s.generate(prompt)
	if err != nil {
		return "", fmt.Errorf("ai: summarization failed: %w", err)
	}

	return strings.TrimSpace(response), nil
}

// ──────────────────────────────────────────────────────────────────────
// Internal: Ollama API call
// ──────────────────────────────────────────────────────────────────────

func (s *Service) generate(prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	reqBody := ollamaRequest{
		Model:  s.model,
		Prompt: prompt,
		Stream: false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("ai: failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.ollamaURL+"/api/generate", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("ai: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai: failed to connect to Ollama at %s: %w", s.ollamaURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ai: failed to read Ollama response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ai: Ollama returned status %d: %s", resp.StatusCode, string(body))
	}

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return "", fmt.Errorf("ai: failed to parse Ollama response: %w", err)
	}

	return ollamaResp.Response, nil
}

// extractJSON attempts to find and extract a JSON object from a string
// that may contain surrounding text.
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end <= start {
		return s
	}
	return s[start : end+1]
}
