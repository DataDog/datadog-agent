package main

//
// @title           payment-service API
// @version         1.0
// @description     Payment processing service with fraud detection
// @host            localhost:8081
// @BasePath        /api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gensim/payment-service/config"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	// Datadog APM - Direct instrumentation
	gintrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/gin-gonic/gin"
	redistrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/redis/go-redis.v9"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	// Swagger documentation
	_ "github.com/gensim/payment-service/docs"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

var (
	configManager *config.Manager
	redisClient   redis.UniversalClient
	randSrc       = rand.New(rand.NewSource(time.Now().UnixNano()))
)

// ============================================================================
// DOMAIN TYPES
// ============================================================================

// PaymentRequest represents a payment request
type PaymentRequest struct {
	TransactionID string  `json:"transaction_id,omitempty"`
	Amount        float64 `json:"amount" binding:"required,gt=0"`
	Currency      string  `json:"currency" binding:"required"`
	CardNumber    string  `json:"card_number" binding:"required"`
	CardHolder    string  `json:"card_holder" binding:"required"`
	ExpiryDate    string  `json:"expiry_date" binding:"required"`
	CVV           string  `json:"cvv" binding:"required"`
	MerchantID    string  `json:"merchant_id" binding:"required"`
	CustomerID    string  `json:"customer_id,omitempty"`
	Description   string  `json:"description,omitempty"`
}

// PaymentResponse represents a payment response
type PaymentResponse struct {
	TransactionID string  `json:"transaction_id"`
	Status        string  `json:"status"`
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
	MerchantID    string  `json:"merchant_id"`
	Message       string  `json:"message,omitempty"`
	FraudScore    float64 `json:"fraud_score,omitempty"`
	ProcessedAt   string  `json:"processed_at"`
}

// ReportRequest represents a compliance report request
type ReportRequest struct {
	ReportType string `json:"report_type" binding:"required"`
	StartDate  string `json:"start_date" binding:"required"`
	EndDate    string `json:"end_date" binding:"required"`
	MerchantID string `json:"merchant_id,omitempty"`
	Format     string `json:"format,omitempty"`
}

// ReportResponse represents a compliance report response
type ReportResponse struct {
	ReportID    string  `json:"report_id"`
	Status      string  `json:"status"`
	ReportType  string  `json:"report_type"`
	StartDate   string  `json:"start_date"`
	EndDate     string  `json:"end_date"`
	GeneratedAt string  `json:"generated_at"`
	DownloadURL string  `json:"download_url,omitempty"`
	RecordCount int     `json:"record_count"`
	TotalAmount float64 `json:"total_amount"`
}

// FraudCheckResult represents the result of a fraud check
type FraudCheckResult struct {
	IsFraudulent bool    `json:"is_fraudulent"`
	Score        float64 `json:"score"`
	Reason       string  `json:"reason,omitempty"`
}

// ============================================================================
// STANDARD UTILITIES
// ============================================================================

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// DogStatsD client for metrics
func statsdAddr() string {
	return getEnv("DD_AGENT_HOST", "dd-agent") + ":" + getEnv("DD_DOGSTATSD_PORT", "8125")
}

func statsdSend(line string) {
	conn, err := net.DialTimeout("udp", statsdAddr(), 100*time.Millisecond)
	if err != nil {
		return
	}
	defer conn.Close()
	_, _ = fmt.Fprint(conn, line)
}

func metricCount(name string, val int, tags []string) {
	tagStr := ""
	if len(tags) > 0 {
		tagStr = "|#" + strings.Join(tags, ",")
	}
	statsdSend(fmt.Sprintf("%s:%d|c%s", name, val, tagStr))
}

func metricTiming(name string, ms float64, tags []string) {
	tagStr := ""
	if len(tags) > 0 {
		tagStr = "|#" + strings.Join(tags, ",")
	}
	statsdSend(fmt.Sprintf("%s:%0.3f|ms%s", name, ms, tagStr))
}

func metricGauge(name string, val float64, tags []string) {
	tagStr := ""
	if len(tags) > 0 {
		tagStr = "|#" + strings.Join(tags, ",")
	}
	statsdSend(fmt.Sprintf("%s:%0.3f|g%s", name, val, tagStr))
}

// Structured logging with Datadog trace context
func logJSON(c *gin.Context, level string, msg string, extra map[string]any) {
	entry := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"level":     strings.ToUpper(level),
		"service":   getEnv("DD_SERVICE", "payment-service"),
		"message":   msg,
	}
	// Get Datadog span context from request
	if c != nil {
		span, ok := tracer.SpanFromContext(c.Request.Context())
		if ok && span != nil {
			entry["dd.trace_id"] = fmt.Sprintf("%d", span.Context().TraceID())
			entry["dd.span_id"] = fmt.Sprintf("%d", span.Context().SpanID())
		}
	}
	for k, v := range extra {
		entry[k] = v
	}
	b, _ := json.Marshal(entry)
	fmt.Println(string(b))
}

func initTracer() func() {
	serviceName := os.Getenv("DD_SERVICE")
	if serviceName == "" {
		serviceName = "payment-service"
	}

	tracer.Start(
		tracer.WithService(serviceName),
		tracer.WithEnv(getEnv("DD_ENV", "development")),
		tracer.WithServiceVersion(getEnv("DD_VERSION", "1.0.0")),
		tracer.WithLogStartup(true),
	)

	log.Printf("Datadog tracer initialized for service: %s", serviceName)

	return func() {
		tracer.Stop()
	}
}

// Initialize Redis client for fraud-db
func initRedis() (redis.UniversalClient, error) {
	// Parse FRAUD_DB_URL - it's set as http://fraud-db:6379 but Redis uses redis:// or direct connection
	fraudDBURL := getEnv("FRAUD_DB_URL", "http://fraud-db:6379")
	// Extract host:port from the URL
	host := "fraud-db:6379"
	if strings.Contains(fraudDBURL, "://") {
		parts := strings.SplitN(fraudDBURL, "://", 2)
		if len(parts) == 2 {
			host = parts[1]
		}
	}

	// Create a traced Redis client using redistrace.NewClient
	// This automatically instruments all Redis commands with Datadog APM
	client := redistrace.NewClient(
		&redis.Options{
			Addr:         host,
			Password:     "", // no password
			DB:           0,  // use default DB
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
			PoolSize:     10,
		},
		redistrace.WithServiceName("fraud-db"),
	)

	// Test connection with retries
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err := client.Ping(ctx).Result()
		cancel()
		if err == nil {
			log.Printf("Successfully connected to Redis (fraud-db) at %s", host)
			return client, nil
		}
		log.Printf("Failed to connect to Redis (attempt %d/%d): %v", i+1, maxRetries, err)
		time.Sleep(2 * time.Second)
	}

	return client, fmt.Errorf("failed to connect to Redis after %d attempts", maxRetries)
}

func main() {
	// Initialize Datadog Tracer
	shutdown := initTracer()
	defer shutdown()

	// Initialize Redis client for fraud detection
	var err error
	redisClient, err = initRedis()
	if err != nil {
		log.Printf("Warning: Redis initialization failed: %v (fraud checks will be degraded)", err)
	}

	// Initialize Config Manager (etcd-backed; also supports chaos toggles if present)
	configManager, err = config.NewManager()
	if err != nil {
		log.Printf("Failed to initialize config manager: %v", err)
	} else {
		defer configManager.Close()
		log.Println("Config manager enabled")
	}

	// Set Gin mode
	if mode := os.Getenv("GIN_MODE"); mode != "" {
		gin.SetMode(mode)
	}

	// Create Gin router
	r := gin.Default()

	// Add Datadog APM middleware for automatic tracing
	r.Use(gintrace.Middleware(getEnv("DD_SERVICE", "payment-service")))

	// CORS middleware
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Health check endpoint
	r.GET("/health", healthCheck)

	// Swagger documentation endpoint - access at /swagger/index.html
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// ============================================================================
	// API ROUTES
	// ============================================================================

	// Payment endpoint - process_payment flow
	r.POST("/pay", processPayment)

	// Internal compliance report endpoint - compliance_report flow
	r.POST("/internal/generate-report", generateReport)

	port := getEnv("PORT", "8081")

	log.Printf("Starting payment-service on port %s", port)
	log.Fatal(r.Run(":" + port))
}

// Health check endpoint
// @Summary      Health check
// @Description  Returns service health status
// @Tags         health
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Router       /health [get]
func healthCheck(c *gin.Context) {
	start := time.Now()

	status := gin.H{
		"status":  "healthy",
		"service": getEnv("DD_SERVICE", "payment-service"),
	}

	// Check Redis connectivity
	if redisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		_, err := redisClient.Ping(ctx).Result()
		cancel()
		if err != nil {
			status["fraud_db"] = "unhealthy"
			status["status"] = "degraded"
		} else {
			status["fraud_db"] = "healthy"
		}
	}

	dur := time.Since(start).Seconds() * 1000
	logJSON(c, "info", fmt.Sprintf("Health check - %.2fms", dur), map[string]any{"duration_ms": dur})

	c.JSON(http.StatusOK, status)
}

// Process payment - handle_transaction operation
// @Summary      Process a payment
// @Description  Process a payment transaction with fraud detection
// @Tags         payments
// @Accept       json
// @Produce      json
// @Param        payment  body      PaymentRequest  true  "Payment Request"
// @Success      200      {object}  PaymentResponse
// @Failure      400      {object}  map[string]interface{}
// @Failure      402      {object}  map[string]interface{}
// @Failure      500      {object}  map[string]interface{}
// @Router       /pay [post]
func processPayment(c *gin.Context) {
	start := time.Now()
	ctx := c.Request.Context()

	// Check config/chaos disruptions
	if configManager != nil {
		if configManager.ShouldFail("payment-service_server_error") {
			metricCount("payment.process.errors", 1, []string{"error:chaos"})
			logJSON(c, "error", "Payment processing failed due to chaos injection", map[string]any{"error": "chaos_injection"})
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "Service temporarily unavailable"})
			return
		}
		configManager.ApplyDelay("payment-service_server_slow")
	}

	var req PaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		metricCount("payment.process.errors", 1, []string{"error:validation"})
		logJSON(c, "warn", "Invalid payment request", map[string]any{"error": err.Error()})
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation_error", "message": err.Error()})
		return
	}

	// Generate transaction ID if not provided
	if req.TransactionID == "" {
		req.TransactionID = uuid.New().String()
	}

	logJSON(c, "info", "Processing payment", map[string]any{
		"transaction_id": req.TransactionID,
		"amount":         req.Amount,
		"currency":       req.Currency,
		"merchant_id":    req.MerchantID,
	})

	// Perform fraud check against fraud-db (Redis)
	fraudResult := checkFraud(ctx, c, req)

	if fraudResult.IsFraudulent {
		metricCount("payment.process.rejected", 1, []string{"reason:fraud"})
		metricTiming("payment.process.latency", time.Since(start).Seconds()*1000, []string{"status:rejected"})
		logJSON(c, "warn", "Payment rejected due to fraud detection", map[string]any{
			"transaction_id": req.TransactionID,
			"fraud_score":    fraudResult.Score,
			"reason":         fraudResult.Reason,
		})
		c.JSON(http.StatusPaymentRequired, PaymentResponse{
			TransactionID: req.TransactionID,
			Status:        "rejected",
			Amount:        req.Amount,
			Currency:      req.Currency,
			MerchantID:    req.MerchantID,
			Message:       "Transaction declined: " + fraudResult.Reason,
			FraudScore:    fraudResult.Score,
			ProcessedAt:   time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	// Simulate payment processing
	// In a real system, this would call payment gateway
	processingTime := time.Duration(rangeInt(10, 50)) * time.Millisecond
	time.Sleep(processingTime)

	// Record successful transaction in Redis for future fraud detection
	if redisClient != nil {
		recordTransaction(ctx, req)
	}

	dur := time.Since(start).Seconds() * 1000
	metricCount("payment.process.success", 1, []string{"currency:" + req.Currency})
	metricTiming("payment.process.latency", dur, []string{"status:success"})
	metricGauge("payment.amount", req.Amount, []string{"currency:" + req.Currency, "merchant_id:" + req.MerchantID})

	logJSON(c, "info", fmt.Sprintf("Payment processed successfully - %.2fms", dur), map[string]any{
		"transaction_id": req.TransactionID,
		"amount":         req.Amount,
		"currency":       req.Currency,
		"duration_ms":    dur,
	})

	c.JSON(http.StatusOK, PaymentResponse{
		TransactionID: req.TransactionID,
		Status:        "approved",
		Amount:        req.Amount,
		Currency:      req.Currency,
		MerchantID:    req.MerchantID,
		Message:       "Payment processed successfully",
		FraudScore:    fraudResult.Score,
		ProcessedAt:   time.Now().UTC().Format(time.RFC3339),
	})
}

// checkFraud performs fraud detection using fraud-db (Redis)
func checkFraud(ctx context.Context, c *gin.Context, req PaymentRequest) FraudCheckResult {
	start := time.Now()

	// Default result - not fraudulent
	result := FraudCheckResult{
		IsFraudulent: false,
		Score:        0.0,
	}

	if redisClient == nil {
		logJSON(c, "warn", "Fraud check skipped - Redis unavailable", nil)
		return result
	}

	// Create a child span for fraud check
	span, ctx := tracer.StartSpanFromContext(ctx, "fraud.check",
		tracer.ResourceName("check_fraud"),
		tracer.ServiceName(getEnv("DD_SERVICE", "payment-service")),
	)
	defer span.Finish()

	// Check if card is in blocklist
	cardKey := fmt.Sprintf("blocklist:card:%s", maskCardNumber(req.CardNumber))
	blocked, err := redisClient.Exists(ctx, cardKey).Result()
	if err != nil {
		logJSON(c, "error", "Failed to check card blocklist", map[string]any{"error": err.Error()})
		span.SetTag("error", true)
		span.SetTag("error.message", err.Error())
	} else if blocked > 0 {
		result.IsFraudulent = true
		result.Score = 1.0
		result.Reason = "Card is blocked"
		span.SetTag("fraud.detected", true)
		span.SetTag("fraud.reason", "blocked_card")
		return result
	}

	// Check transaction velocity - count recent transactions for this card
	velocityKey := fmt.Sprintf("velocity:card:%s", maskCardNumber(req.CardNumber))
	count, err := redisClient.Incr(ctx, velocityKey).Result()
	if err != nil {
		logJSON(c, "error", "Failed to check transaction velocity", map[string]any{"error": err.Error()})
	} else {
		// Set expiry on velocity key (1 hour window)
		redisClient.Expire(ctx, velocityKey, 1*time.Hour)

		// High velocity = suspicious
		if count > 10 {
			result.Score = float64(count) / 20.0
			if result.Score > 1.0 {
				result.Score = 1.0
			}
			if count > 20 {
				result.IsFraudulent = true
				result.Reason = "Transaction velocity too high"
				span.SetTag("fraud.detected", true)
				span.SetTag("fraud.reason", "high_velocity")
			}
		}
	}

	// Check for high-risk amount thresholds
	if req.Amount > 10000 {
		result.Score += 0.3
		if result.Score > 0.8 {
			result.IsFraudulent = true
			result.Reason = "High-risk transaction amount"
			span.SetTag("fraud.detected", true)
			span.SetTag("fraud.reason", "high_amount")
		}
	}

	// Normalize score
	if result.Score > 1.0 {
		result.Score = 1.0
	}

	dur := time.Since(start).Seconds() * 1000
	metricTiming("fraud.check.latency", dur, []string{fmt.Sprintf("fraudulent:%t", result.IsFraudulent)})
	span.SetTag("fraud.score", result.Score)
	span.SetTag("fraud.is_fraudulent", result.IsFraudulent)

	logJSON(c, "info", fmt.Sprintf("Fraud check completed - %.2fms", dur), map[string]any{
		"fraud_score":   result.Score,
		"is_fraudulent": result.IsFraudulent,
		"duration_ms":   dur,
	})

	return result
}

// recordTransaction records a successful transaction for future fraud detection
func recordTransaction(ctx context.Context, req PaymentRequest) {
	if redisClient == nil {
		return
	}

	// Store transaction data
	txKey := fmt.Sprintf("tx:%s", req.TransactionID)
	txData := map[string]interface{}{
		"amount":      req.Amount,
		"currency":    req.Currency,
		"merchant_id": req.MerchantID,
		"card_hash":   maskCardNumber(req.CardNumber),
		"timestamp":   time.Now().Unix(),
	}
	txJSON, _ := json.Marshal(txData)
	redisClient.Set(ctx, txKey, txJSON, 24*time.Hour)

	// Update merchant transaction count
	merchantKey := fmt.Sprintf("merchant:tx_count:%s", req.MerchantID)
	redisClient.Incr(ctx, merchantKey)
	redisClient.Expire(ctx, merchantKey, 30*24*time.Hour) // 30 days
}

// Generate compliance report
// @Summary      Generate compliance report
// @Description  Generate a compliance report for the specified period
// @Tags         reports
// @Accept       json
// @Produce      json
// @Param        report  body      ReportRequest  true  "Report Request"
// @Success      200     {object}  ReportResponse
// @Failure      400     {object}  map[string]interface{}
// @Failure      500     {object}  map[string]interface{}
// @Router       /internal/generate-report [post]
func generateReport(c *gin.Context) {
	start := time.Now()
	ctx := c.Request.Context()

	// Check config/chaos disruptions
	if configManager != nil {
		if configManager.ShouldFail("payment-service_report_error") {
			metricCount("report.generate.errors", 1, []string{"error:chaos"})
			logJSON(c, "error", "Report generation failed due to chaos injection", map[string]any{"error": "chaos_injection"})
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": "Report generation failed"})
			return
		}
		configManager.ApplyDelay("payment-service_report_slow")
	}

	var req ReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		metricCount("report.generate.errors", 1, []string{"error:validation"})
		logJSON(c, "warn", "Invalid report request", map[string]any{"error": err.Error()})
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation_error", "message": err.Error()})
		return
	}

	// Set default format
	if req.Format == "" {
		req.Format = "pdf"
	}

	reportID := uuid.New().String()

	logJSON(c, "info", "Generating compliance report", map[string]any{
		"report_id":   reportID,
		"report_type": req.ReportType,
		"start_date":  req.StartDate,
		"end_date":    req.EndDate,
	})

	// Create a child span for report generation
	span, _ := tracer.StartSpanFromContext(ctx, "report.generate",
		tracer.ResourceName("generate_pdf"),
		tracer.ServiceName(getEnv("DD_SERVICE", "payment-service")),
	)
	defer span.Finish()

	// Simulate report generation (PDF generation)
	// In a real system, this would query the database and generate actual reports
	processingTime := time.Duration(rangeInt(100, 500)) * time.Millisecond
	time.Sleep(processingTime)

	// Get transaction stats from Redis if available
	recordCount := rangeInt(100, 5000)
	totalAmount := rangeFloat(10000, 1000000)

	if redisClient != nil {
		// Try to get actual merchant transaction count
		if req.MerchantID != "" {
			merchantKey := fmt.Sprintf("merchant:tx_count:%s", req.MerchantID)
			count, err := redisClient.Get(ctx, merchantKey).Int()
			if err == nil && count > 0 {
				recordCount = count
			}
		}
	}

	dur := time.Since(start).Seconds() * 1000
	metricCount("report.generate.success", 1, []string{"type:" + req.ReportType, "format:" + req.Format})
	metricTiming("report.generate.latency", dur, []string{"status:success"})

	span.SetTag("report.id", reportID)
	span.SetTag("report.type", req.ReportType)
	span.SetTag("report.record_count", recordCount)

	logJSON(c, "info", fmt.Sprintf("Report generated successfully - %.2fms", dur), map[string]any{
		"report_id":    reportID,
		"record_count": recordCount,
		"duration_ms":  dur,
	})

	c.JSON(http.StatusOK, ReportResponse{
		ReportID:    reportID,
		Status:      "completed",
		ReportType:  req.ReportType,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		DownloadURL: fmt.Sprintf("/reports/%s.%s", reportID, req.Format),
		RecordCount: recordCount,
		TotalAmount: totalAmount,
	})
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

func maskCardNumber(cardNumber string) string {
	// Return a hash-like representation for privacy
	if len(cardNumber) < 4 {
		return "****"
	}
	return "****" + cardNumber[len(cardNumber)-4:]
}

func randomItem(items ...string) string {
	if len(items) == 0 {
		return ""
	}
	return items[randSrc.Intn(len(items))]
}

func rangeInt(min, max int) int {
	if max <= min {
		return min
	}
	return min + randSrc.Intn(max-min+1)
}

func rangeFloat(min, max float64) float64 {
	if max <= min {
		return min
	}
	return min + randSrc.Float64()*(max-min)
}
