package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"inventory-system/internal/config"
	"inventory-system/internal/database"
	"inventory-system/internal/domain"                   // For domain errors, if main needs to know them
	analyticshandler "inventory-system/internal/handler" // Alias to avoid name collision
	itemhandler "inventory-system/internal/handler"      // Alias for clarity
	wshandler "inventory-system/internal/handler"        // Alias for clarity
	"inventory-system/internal/realtime"
	analyticsrepo "inventory-system/internal/repository" // If analytics had a separate repo
	itemrepo "inventory-system/internal/repository"
	analyticsservice "inventory-system/internal/service"
	itemservice "inventory-system/internal/service"
	"inventory-system/pkg/httputil" // For custom HTTP error handler

	"github.com/go-playground/validator"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// CustomValidator for Echo to use go-playground/validator
type CustomValidator struct {
	validator *validator.Validate
}

func (cv *CustomValidator) Validate(i interface{}) error {
	if err := cv.validator.Struct(i); err != nil {
		// Optionally, you can return a custom error type here that your HTTPErrorHandler can understand.
		// For now, Echo's default error handler will catch this or we handle it in handlers.
		return err
	}
	return nil
}


func main() {
	// --- Configuration ---
	cfg, err := config.LoadConfig(".") // Load from .env or environment
	if err != nil {
		log.Fatalf("FATAL: Could not load config: %v", err)
	}

	// --- Database ---
	dbPool, err := database.ConnectPostgres(cfg.DBSource)
	if err != nil {
		log.Fatalf("FATAL: Could not connect to database: %v", err)
	}
	defer func() {
		log.Println("Closing database connection pool...")
		dbPool.Close()
	}()

	// Run Migrations
	// In a production setup, you might run migrations as a separate step/command
	// or use a more sophisticated migration tool integrated into your deployment pipeline.
	log.Println("Attempting to run database migrations...")
	database.RunMigrations(cfg.MigrationURL, cfg.DBSource) // Uses DSN from config

	// --- Echo Instance ---
	e := echo.New()

	// --- Middleware ---
	e.Use(middleware.RequestID()) // Add request ID to context and response header
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{ // Structured logging
		Format: `{"time":"${time_rfc3339_nano}","id":"${id}","remote_ip":"${remote_ip}",` +
			`"host":"${host}","method":"${method}","uri":"${uri}","user_agent":"${user_agent}",` +
			`"status":${status},"error":"${error}","latency":${latency},"latency_human":"${latency_human}"` +
			`,"bytes_in":${bytes_in},"bytes_out":${bytes_out}}` + "\n",
	}))
	e.Use(middleware.Recover()) // Recover from panics anywhere in the chain
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"http://localhost:3000", "http://localhost:5173", cfg.FrontendURL}, // Adjust for your frontend URL
		AllowMethods: []string{http.MethodGet, http.MethodPut, http.MethodPost, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
	}))
	
	// Set custom validator
	// We instantiate the validator within the handler, but if you want Echo's default binding/validation
	// to use it universally, you'd set it like this. Our handlers call validate.StructCtx directly.
	// e.Validator = &CustomValidator{validator: validator.New()}

	// Custom HTTP Error Handler
	// This allows us to centralize how errors (especially those from validation or unhandled ones)
	// are converted into our httputil.HTTPError format.
	e.HTTPErrorHandler = customHTTPErrorHandler

	// --- Real-time Hub ---
	hub := realtime.NewHub()
	go hub.Run() // Start the hub in its own goroutine
	log.Println("Realtime Hub started.")

	// --- Dependency Injection (Repositories, Services, Handlers) ---
	// Item
	itemRepository := itemrepo.NewPgItemRepository(dbPool)
	itemSvc := itemservice.NewItemService(itemRepository, hub) // Pass hub to item service
	itemHdlr := itemhandler.NewItemHandler(itemSvc)

	// Analytics (ItemRepository is used for analytics queries as per our design)
	analyticsSvc := analyticsservice.NewAnalyticsService(itemRepository)
	analyticsHdlr := analyticshandler.NewAnalyticsHandler(analyticsSvc)

	// WebSocket
	wsHdlr := wshandler.NewWebSocketHandler(hub)

	// --- Routes ---
	e.GET("/", healthCheckHandler) // Basic health check

	apiV1 := e.Group("/api/v1")

	// Item routes
	itemsGroup := apiV1.Group("/items")
	itemsGroup.POST("", itemHdlr.CreateItem)
	itemsGroup.GET("", itemHdlr.GetItems)
	itemsGroup.GET("/:id", itemHdlr.GetItemByID)
	itemsGroup.PUT("/:id", itemHdlr.UpdateItem)
	itemsGroup.DELETE("/:id", itemHdlr.DeleteItem)

	// Analytics routes
	analyticsGroup := apiV1.Group("/analytics")
	analyticsGroup.GET("/stock-value", analyticsHdlr.GetTotalStockValue)
	analyticsGroup.GET("/low-stock", analyticsHdlr.GetLowStockItems)
	analyticsGroup.GET("/most-valuable", analyticsHdlr.GetMostValuableItems)

	// WebSocket route
	// Note: The path for WebSocket is typically outside /api/v1, but can be anywhere.
	e.GET("/ws/stock-updates", wsHdlr.HandleConnections)

	// --- Start Server with Graceful Shutdown ---
	// Start server in a goroutine so that it doesn't block.
	go func() {
		log.Printf("Starting server on port %s", cfg.ServerPort)
		if err := e.Start(":" + cfg.ServerPort); err != nil && !errors.Is(err, http.ErrServerClosed) {
			e.Logger.Fatal("shutting down the server unexpectedly:", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server with a timeout.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM) // Listen for SIGINT and SIGTERM
	<-quit                                             // Block until a signal is received

	log.Println("Shutdown signal received, initiating graceful shutdown...")

	// Create a context with a timeout for the shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // 10-second timeout
	defer cancel()

	// Attempt to gracefully shut down the Echo server.
	if err := e.Shutdown(ctx); err != nil {
		e.Logger.Fatal("Error during server shutdown:", err)
	}

	log.Println("Server gracefully shut down.")
}

// healthCheckHandler is a simple handler for health checks.
func healthCheckHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, echo.Map{
		"status":  "ok",
		"message": "Inventory System API is running!",
		"version": "1.0.0", // Example version
		"time":    time.Now().Format(time.RFC3339),
	})
}

// customHTTPErrorHandler provides centralized error handling.
func customHTTPErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return // If response already sent, do nothing
	}

	var he *httputil.HTTPError // Our custom error type
	var echoHE *echo.HTTPError // Echo's built-in HTTPError

	if errors.As(err, &he) {
		// This is already our structured HTTPError, just send it
		if c.Request().Method == http.MethodHead { // Echo's default behavior
			_ = c.NoContent(he.StatusCode)
		} else {
			_ = httputil.SendErrorResponse(c, he) // Use our sender
		}
		return
	} else if errors.As(err, &echoHE) {
		// Convert Echo's HTTPError to our httputil.HTTPError format
		// This handles errors from Echo's internals (e.g., routing not found, method not allowed)
		appErr := httputil.NewHTTPError(echoHE.Code, echoHE.Message.(string))
		// If echoHE.Internal is not nil, it might contain the original error.
		// You could log it or add parts of it to details if safe.
		// log.Printf("Echo internal error: %v", echoHE.Internal)
		_ = httputil.SendErrorResponse(c, appErr)
		return
	}

	// Handle validation errors from go-playground/validator if not handled in specific handlers
	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		validationErrors := itemhandler.parseValidationErrors(err) // Use the parser from itemhandler
		appErr := httputil.ValidationError("Input validation failed", validationErrors)
		_ = httputil.SendErrorResponse(c, appErr)
		return
	}
	
	// Handle domain-specific errors that might bubble up if not caught by handlers
	// This provides a last line of defense for known error types.
	if errors.Is(err, domain.ErrItemNotFound) || errors.Is(err, domain.ErrRepositoryNotFound) {
		_ = httputil.SendErrorResponse(c, httputil.NotFoundError(err.Error()))
		return
	}
	if errors.Is(err, domain.ErrSKUAlreadyExists) || errors.Is(err, domain.ErrRepositoryDuplicateEntry) {
		_ = httputil.SendErrorResponse(c, httputil.ConflictError(err.Error()))
		return
	}
	if errors.Is(err, domain.ErrInvalidInput) || errors.Is(err, domain.ErrInvalidItemID) {
		_ = httputil.SendErrorResponse(c, httputil.BadRequestError(err.Error()))
		return
	}


	// For any other unhandled errors, return a generic 500
	log.Printf("Unhandled error: %v", err) // Log the full error for debugging
	appErr := httputil.InternalServerError("An unexpected internal error occurred.")
	_ = httputil.SendErrorResponse(c, appErr)
}
