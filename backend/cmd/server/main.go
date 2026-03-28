// Package main is the entry point for the MedConnect Oriental backend server.
// It wires up configuration, database, crypto, AI, WhatsApp, middleware, and routes.
package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"medconnect-oriental/backend/internal/ai"
	"medconnect-oriental/backend/internal/api"
	"medconnect-oriental/backend/internal/crypto"
	"medconnect-oriental/backend/internal/middleware"
	"medconnect-oriental/backend/internal/models"
	"medconnect-oriental/backend/internal/repository"
	"medconnect-oriental/backend/internal/service"
	"medconnect-oriental/backend/internal/websocket"
)

func main() {
	// Attempt to load .env from the parent directory where docker-compose is, or current directory
	_ = godotenv.Load("../.env")
	_ = godotenv.Load(".env")

	// ── Load Configuration ───────────────────────────────────
	dbDSN := getEnv("DB_DSN", "host=localhost user=medadmin password=securepass123 dbname=medconnect port=5432 sslmode=disable")
	aesKey := getEnv("AES_KEY", "")
	jwtSecret := getEnv("JWT_SECRET", "")
	serverPort := getEnv("SERVER_PORT", "3000")
	ollamaURL := getEnv("OLLAMA_URL", "http://localhost:11434")
	ollamaModel := getEnv("OLLAMA_MODEL", "llama3:8b-instruct-q4_K_M")
	waURL := getEnv("WA_URL", "http://localhost:8080")
	waToken := getEnv("WA_TOKEN", "")
	waInstance := getEnv("WA_INSTANCE", "medconnect")

	// Validate required security configuration
	if aesKey == "" {
		log.Fatal("FATAL: AES_KEY environment variable is required")
	}
	if jwtSecret == "" {
		log.Fatal("FATAL: JWT_SECRET environment variable is required - no default allowed for security")
	}
	if len(jwtSecret) < 32 {
		log.Fatal("FATAL: JWT_SECRET must be at least 32 characters for security")
	}

	// ── Initialize Crypto ────────────────────────────────────
	aesCrypto := crypto.MustNewAESCrypto(aesKey)
	log.Println("🔐 AES-256-GCM encryption initialized")

	// ── Connect Database ─────────────────────────────────────
	db, err := repository.ConnectDatabase(dbDSN)
	if err != nil {
		log.Fatalf("FATAL: %v", err)
	}

	// Migrate Notification model (added in Phase 2)
	if err := db.AutoMigrate(&models.Notification{}); err != nil {
		log.Fatalf("FATAL: Failed to migrate Notification model: %v", err)
	}

	// ── Seed Test Accounts ───────────────────────────────────
	if err := repository.SeedTestAccounts(db); err != nil {
		log.Fatalf("FATAL: Failed to seed test accounts: %v", err)
	}

	// ── Initialize Services ──────────────────────────────────
	aiService := ai.NewService(ollamaURL, ollamaModel)
	log.Printf("🤖 AI service configured (Ollama: %s, Model: %s)", ollamaURL, ollamaModel)

	var waService *service.WhatsAppService
	if waToken != "" {
		waService = service.NewWhatsAppService(waURL, waToken, waInstance)
		log.Printf("📱 WhatsApp service configured (Evolution API: %s)", waURL)
	} else {
		log.Println("⚠️  WhatsApp service disabled (WA_TOKEN not set)")
	}

	notifService := service.NewNotificationService(db)

	// ── Initialize WebSocket Hub ─────────────────────────────
	wsHub := websocket.NewHub()
	go wsHub.Run()
	log.Println("🔌 WebSocket hub initialized")

	// ── Initialize Decryption Cache ───────────────────────────
	decryptionCache := service.NewDecryptionCache(service.DefaultCacheConfig())
	log.Println("💾 Decryption cache initialized (TTL: 5 minutes, Max entries: 10000)")

	// ── Handler Context ──────────────────────────────────────
	h := &api.HandlerContext{
		DB:              db,
		Crypto:          aesCrypto,
		AI:              aiService,
		WhatsApp:        waService,
		Notification:    notifService,
		DecryptionCache: decryptionCache,
		WSHub:           wsHub,
	}

	// ── Setup Gin Router ─────────────────────────────────────
	router := gin.Default()

	// CORS configuration from environment
	allowedOrigins := getEnv("ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:3000")
	allowedOriginsList := strings.Split(allowedOrigins, ",")

	// Global CORS Middleware with proper origin validation
	router.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Validate origin against allowed list
		originAllowed := false
		for _, allowedOrigin := range allowedOriginsList {
			trimmedOrigin := strings.TrimSpace(allowedOrigin)
			if origin == trimmedOrigin {
				originAllowed = true
				break
			}
		}

		if originAllowed {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, PATCH, DELETE")
		c.Writer.Header().Set("Access-Control-Max-Age", "86400") // 24 hours

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Security headers middleware — adds protective HTTP headers
	router.Use(middleware.SecurityHeadersMiddleware())

	// Global audit middleware — logs every request
	router.Use(middleware.AuditMiddleware(db))

	// ── Public Routes ────────────────────────────────────────
	// Login endpoint with rate limiting (10 attempts per minute)
	loginLimiter := middleware.NewRateLimiter(10, time.Minute)
	router.POST("/api/login", loginLimiter.Middleware(), loginHandler(db, jwtSecret))

	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "healthy",
			"service": "medconnect-oriental",
		})
	})

	// ── WebSocket Route ─────────────────────────────────────
	router.GET("/ws", websocket.GinWebSocket(wsHub, jwtSecret))

	// ── Protected Routes ─────────────────────────────────────
	authorized := router.Group("/api")
	authorized.Use(middleware.JWTAuthMiddleware(jwtSecret))
	{
		// Directory — accessible to all authenticated users
		authorized.GET("/directory", h.GetDirectory)

		// Referral detail — accessible to CHU_DOC (own dept) + LEVEL_2_DOC (own referrals) + admins
		authorized.GET("/referrals/:id", h.GetReferral)

		// Notification mark-as-read — all authenticated
		authorized.PATCH("/notifications/:id/read", h.MarkNotificationRead)

		// Attachment retrieval — all authenticated
		authorized.GET("/attachments/:id", h.GetAttachment)

		// ── Level 2 Doctor Routes ────────────────────────────
		level2 := authorized.Group("")
		level2.Use(middleware.RBACMiddleware(models.RoleLevel2Doc))
		{
			level2.POST("/referrals", h.CreateReferral)
			level2.POST("/referrals/suggest", h.SuggestDepartment)
			level2.GET("/notifications", h.GetNotifications)
			level2.POST("/referrals/:id/attachments", h.UploadAttachments)
		}

		// ── CHU Doctor Routes ────────────────────────────────
		chu := authorized.Group("")
		chu.Use(middleware.RBACMiddleware(models.RoleCHUDoc))
		{
			chu.GET("/queue", h.GetQueue)
			chu.PATCH("/referrals/:id/schedule", h.ScheduleReferral)
			chu.PATCH("/referrals/:id/redirect", h.RedirectReferral)
			chu.PATCH("/referrals/:id/deny", h.DenyReferral)
			chu.PATCH("/referrals/:id/reschedule", h.RescheduleReferral)
			chu.PATCH("/referrals/:id/cancel", h.CancelReferral)
		}

		// ── Analyst Routes ───────────────────────────────────
		analyst := authorized.Group("/analyst")
		analyst.Use(middleware.RBACMiddleware(models.RoleAnalyst, models.RoleSuperAdmin))
		{
			analyst.GET("/stats/departments", h.GetAdminDepartments)
			analyst.GET("/stats/doctors", h.GetAnalystDoctorStats)
		}

		// Combined History Route (Level 2 & CHU Doc read from same handler internally routing based on role)
		authorized.GET("/history", h.GetReferralHistory)

		// ── SIH Interoperability Routes (FHIR R4 Pull API) ─────────────
		fhir := authorized.Group("/fhir/R4")
		{
			fhir.GET("/Patient/:id", h.GetFHIRPatient)
			fhir.GET("/ServiceRequest/:id", h.GetFHIRServiceRequest)
			fhir.GET("/Encounter/:id", h.GetFHIREncounter)
			fhir.GET("/referrals/:id", h.GetFHIRReferralBundle)
		}

		// ── Admin Routes ─────────────────────────────────────
		admin := authorized.Group("/admin")
		admin.Use(middleware.RBACMiddleware(models.RoleSuperAdmin))
		{
			admin.GET("/stats", h.GetAdminStats)

			admin.GET("/users", h.GetUsers)
			admin.POST("/users", h.CreateUser)
			admin.DELETE("/users/:id", h.DeleteUser)

			admin.GET("/departments", h.GetAdminDepartments)
			admin.POST("/departments", h.CreateDepartment)
			admin.PATCH("/departments/:id", h.UpdateDepartment)
			admin.DELETE("/departments/:id", h.DeleteDepartment)

			admin.GET("/audit-logs", h.GetAuditLogs)
			admin.GET("/audit-logs/export", h.GetAuditLogExport)
			admin.GET("/audit-logs/users", h.GetUsersForFilter)
			admin.GET("/audit-logs/actions", h.GetActionsForFilter)

			admin.GET("/referrals/export", h.GetReferralsExport)
		}
	}

	// ── Start Server ─────────────────────────────────────────
	log.Printf("🚀 MedConnect Oriental server starting on :%s", serverPort)
	log.Printf("   📋 Routes: %d", len(router.Routes()))
	if err := router.Run(":" + serverPort); err != nil {
		log.Fatalf("FATAL: Server failed to start: %v", err)
	}
}

// ──────────────────────────────────────────────────────────────────────
// Login Handler
// ──────────────────────────────────────────────────────────────────────

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func loginHandler(db *gorm.DB, jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req loginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Username and password are required"})
			return
		}

		var user models.User
		if err := db.Where("username = ? AND is_active = ?", req.Username, true).First(&user).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
			return
		}

		token, err := middleware.GenerateToken(user, jwtSecret)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"token": token,
			"user": gin.H{
				"id":            user.ID,
				"username":      user.Username,
				"role":          user.Role,
				"facility_name": user.FacilityName,
			},
		})
	}
}

// ──────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
