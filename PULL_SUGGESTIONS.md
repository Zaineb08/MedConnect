# MedConnect - Pull Suggestions Handover

This document summarizes the proposed upgrade work so the team can selectively apply what is needed.

## Context
These changes were implemented on branch `zaineb/suggestions-chu-deploiement` and pushed to fork `Zaineb08/MedConnect`.

## Delivered Scope

### Commit `5251e63`
Title: `feat: add quantized ollama, survival checklist, and offline store-forward`

Includes:
- Task 3: Ollama 4-bit quantized default model (`llama3:8b-instruct-q4_K_M`).
- Task 3: Backend AI HTTP client optimization (timeouts, pooling, transport tuning).
- Task 2: `GenerateSurvivalChecklist(symptoms, dept)` in AI service.
- Task 2: Schedule workflow now appends bilingual survival checklist to WhatsApp notification.
- Task 4: PWA support (`vite-plugin-pwa`) and service worker registration.
- Task 4: Encrypted offline referral queue in IndexedDB (`localforage`).
- Task 4: Store-and-Forward sync when connection comes back.

### Commit `5736f04`
Title: `feat: add FHIR R4 pull APIs and mandatory patient consent`

Includes:
- Task 1: New SIH interoperability route group `/api/fhir/R4`.
- Task 1: FHIR pull endpoints for `Patient`, `ServiceRequest`, `Encounter`, and referral bundle.
- Task 1: Mapping functions from MedConnect models to FHIR-shaped resources.
- Task 5: Referral consent fields in model/DTO.
- Task 5: Mandatory consent validation in `POST /api/referrals`.
- Task 5: Secure audit log entry for consent rejection.
- Task 5: Mandatory consent checkbox in Level 2 referral form.

## Install Commands

### Frontend
```bash
cd frontend
npm install localforage
npm install -D vite-plugin-pwa
```

### Backend (FHIR models library requested by architecture brief)
```bash
cd backend
go get github.com/samply/golang-fhir-models
```

## Validation Status
- Frontend build: passed (`npm run build`).
- PWA assets generated successfully.
- Backend code diagnostics in editor: no errors on modified files.
- Full `go test ./...`: not executed in this environment (Go CLI unavailable here).

## Optional Team Workflow (selective apply)
If maintainers want to apply only one part:

```bash
# Apply only Task 3/2/4
 git cherry-pick 5251e63

# Apply Task 1/5
 git cherry-pick 5736f04
```

## Recommended Review Order
1. Infrastructure + performance (Task 3)
2. Clinical value (Task 2)
3. Offline continuity (Task 4)
4. Compliance (Task 5)
5. SIH interoperability (Task 1)

## Notes for Tomorrow Meeting
- This is a proposal set. Team can merge all, partially merge, or split by commit.
- Prioritize production hardening after merge: secrets rotation, role-scoped FHIR auth, and backend integration tests.
