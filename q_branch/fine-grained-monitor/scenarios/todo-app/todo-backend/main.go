package main

// !!!! LLM NOTE: PLEASE CUSTOMIZE THE RAPID-HTTP SERVER FOR YOUR APPLICATION, AND REMOVE THIS NOTE.
// 1. Define your domain types (structs) below
// 2. Initialize database connection if needed (see initDatabase pattern)
// 3. Define your API routes in main()
// 4. Implement your handlers

//
// @title           todo-backend API
// @version         1.0
// @description     API server for todo-backend
// @host            localhost:8081
// @BasePath        /api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gensim/todo-backend/config"

	// Uncomment for S3 storage if needed
	// "github.com/gensim/todo-backend/storage"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"

	// Datadog APM - Direct instrumentation
	// See: https://docs.datadoghq.com/tracing/trace_collection/automatic_instrumentation/dd_libraries/go/
	sqltrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/database/sql"
	gintrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/gin-gonic/gin"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	// Swagger documentation
	_ "github.com/gensim/todo-backend/docs"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

var (
	configManager *config.Manager
	db            *sql.DB
	randSrc       = rand.New(rand.NewSource(time.Now().UnixNano()))
)

// ============================================================================
// !!!! CUSTOMIZE: Define your domain types here
// ============================================================================
// Example:
// type Item struct {
//     ID        int       `json:"id"`
//     Name      string    `json:"name"`
//     CreatedAt time.Time `json:"created_at"`
// }

// ============================================================================
// STANDARD UTILITIES - Keep these as-is
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
		"service":   getEnv("DD_SERVICE", "todo-backend"),
		"message":   msg,
	}
	// Get Datadog span context from request
	span, ok := tracer.SpanFromContext(c.Request.Context())
	if ok && span != nil {
		entry["dd.trace_id"] = fmt.Sprintf("%d", span.Context().TraceID())
		entry["dd.span_id"] = fmt.Sprintf("%d", span.Context().SpanID())
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
		serviceName = "todo-backend"
	}

	// Start Datadog tracer with options
	// Configuration is automatically read from DD_* environment variables:
	// - DD_AGENT_HOST: Datadog Agent host (default: localhost)
	// - DD_TRACE_AGENT_PORT: APM port (default: 8126)
	// - DD_SERVICE: Service name
	// - DD_ENV: Environment
	// - DD_VERSION: Version
	// See: https://docs.datadoghq.com/tracing/trace_collection/library_config/go/
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

// Database initialization with retry logic
// !!!! CUSTOMIZE: Enable this if your component uses a database
func initDatabase() (*sql.DB, error) {
	// Check for DATABASE_URL first, then construct from individual env vars
	databaseURL := getEnv("DATABASE_URL", "")
	if databaseURL == "" {
		host := getEnv("POSTGRES_HOST", "localhost")
		port := getEnv("POSTGRES_PORT", "5432")
		user := getEnv("POSTGRES_USER", "postgres")
		password := getEnv("POSTGRES_PASSWORD", "password")
		dbname := getEnv("POSTGRES_DB", "app_db")
		databaseURL = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
			user, password, host, port, dbname)
	}

	var database *sql.DB
	var err error
	maxRetries := 30
	sqltrace.Register("postgres", &pq.Driver{}, sqltrace.WithServiceName(getEnv("DD_SERVICE", "todo-backend")+"-db"))
	for i := 0; i < maxRetries; i++ {
		database, err = sqltrace.Open("postgres", databaseURL)
		if err == nil {
			err = database.Ping()
			if err == nil {
				log.Println("Successfully connected to database")
				return database, nil
			}
		}
		log.Printf("Failed to connect to database (attempt %d/%d): %v", i+1, maxRetries, err)
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", maxRetries, err)
}

func main() {
	// Initialize Datadog Tracer
	shutdown := initTracer()
	defer shutdown()

	// !!!! CUSTOMIZE: Uncomment to enable database
	// var err error
	// db, err = initDatabase()
	// if err != nil {
	//     log.Fatalf("Database initialization failed: %v", err)
	// }
	// defer db.Close()

	// Initialize Config Manager (etcd-backed; also supports chaos toggles if present)
	var err error
	configManager, err = config.NewManager()
	if err != nil {
		log.Printf("Failed to initialize config manager: %v", err)
	} else {
		defer configManager.Close()
		log.Println("Config manager enabled (ETCD_ENDPOINTS)")
	}

	// Set Gin mode
	if mode := os.Getenv("GIN_MODE"); mode != "" {
		gin.SetMode(mode)
	}

	// Create Gin router
	r := gin.Default()

	// Add Datadog APM middleware for automatic tracing
	// See: https://docs.datadoghq.com/tracing/trace_collection/dd_libraries/go/
	r.Use(gintrace.Middleware(getEnv("DD_SERVICE", "todo-backend")))

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
	// !!!! CUSTOMIZE: Define your API routes here
	// ============================================================================
	api := r.Group("/api")
	{
		// Example route patterns - replace with your application's routes:
		// api.GET("/items", listItems)
		// api.POST("/items", createItem)
		// api.GET("/items/:id", getItem)
		// api.PUT("/items/:id", updateItem)
		// api.DELETE("/items/:id", deleteItem)

		// Placeholder route - REMOVE after customization
		api.GET("/placeholder", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"message": "Replace this placeholder with your actual API routes",
				"service": getEnv("DD_SERVICE", "todo-backend"),
			})
		})
	}

	port := getEnv("PORT", "8081")

	log.Printf("Starting server on port %s", port)
	log.Fatal(r.Run(":" + port))
}

// Health check endpoint
func healthCheck(c *gin.Context) {
	start := time.Now()

	status := gin.H{
		"status":  "healthy",
		"service": getEnv("DD_SERVICE", "todo-backend"),
	}

	// !!!! CUSTOMIZE: Add database health check if using database
	// if db != nil {
	//     if err := db.Ping(); err != nil {
	//         status["database"] = "unhealthy"
	//         status["status"] = "degraded"
	//     } else {
	//         status["database"] = "healthy"
	//     }
	// }

	dur := time.Since(start).Seconds() * 1000
	logJSON(c, "info", fmt.Sprintf("Health check - %.2fms", dur), map[string]any{"duration_ms": dur})

	c.JSON(http.StatusOK, status)
}

// ============================================================================
// !!!! CUSTOMIZE: Implement your handler functions below
// ============================================================================
// Example handler with proper patterns including Swagger annotations:
//
// @Summary      List items
// @Description  Get all items with optional pagination
// @Tags         items
// @Accept       json
// @Produce      json
// @Param        limit   query     int  false  "Number of items to return" default(50)
// @Param        offset  query     int  false  "Offset for pagination" default(0)
// @Success      200  {object}  map[string]interface{}
// @Failure      500  {object}  map[string]interface{}
// @Router       /items [get]
// func listItems(c *gin.Context) {
//     start := time.Now()
//
//     // Check config/chaos disruptions
//     if configManager != nil {
//         if configManager.ShouldFail("todo-backend_server_error") {
//             metricCount("app.list_items.errors", 1, []string{"error:chaos"})
//             c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
//             return
//         }
//         configManager.ApplyDelay("todo-backend_server_slow")
//     }
//
//     // !!!! CUSTOMIZE: Add your database query here
//     // rows, err := db.Query("SELECT id, name, created_at FROM items ORDER BY created_at DESC LIMIT 50")
//     // ...
//
//     dur := time.Since(start).Seconds() * 1000
//     metricCount("app.list_items.hits", 1, []string{"status:success"})
//     metricTiming("app.list_items.latency", dur, []string{"status:success"})
//     logJSON(c, "info", fmt.Sprintf("GET /api/items - 200 OK %.2fms", dur), map[string]any{"duration_ms": dur})
//
//     c.JSON(http.StatusOK, gin.H{"items": []any{}}) // Replace with actual data
// }
//
// NOTE: After customizing your handlers, run `swag init` to generate the docs/ folder.
// The Swagger UI will be available at /swagger/index.html

// Random helper functions for realistic data generation
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
