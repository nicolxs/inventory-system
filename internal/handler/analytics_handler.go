package handler

import (
	"log"
	"net/http"
	"strconv"

	"inventory-system/internal/domain"
	"inventory-system/pkg/httputil"

	"github.com/labstack/echo/v4"
)

// AnalyticsHandler handles HTTP requests for analytics.
type AnalyticsHandler struct {
	analyticsService domain.AnalyticsService
}

// NewAnalyticsHandler creates a new AnalyticsHandler.
func NewAnalyticsHandler(as domain.AnalyticsService) *AnalyticsHandler {
	return &AnalyticsHandler{analyticsService: as}
}

// GetTotalStockValue godoc
// @Summary Get total stock value
// @Description Calculates the sum of (quantity * price) for all items
// @Tags analytics
// @Produce json
// @Success 200 {object} map[string]float64 "total_value": 12345.67
// @Failure 500 {object} httputil.HTTPError "Internal Server Error"
// @Router /analytics/stock-value [get]
func (h *AnalyticsHandler) GetTotalStockValue(c echo.Context) error {
	value, err := h.analyticsService.CalculateTotalStockValue(c.Request().Context())
	if err != nil {
		log.Printf("GetTotalStockValue: Service error: %v", err)
		return httputil.SendErrorResponse(c, httputil.InternalServerError("Failed to calculate total stock value."))
	}
	return c.JSON(http.StatusOK, echo.Map{"total_value": value})
}

// GetLowStockItems godoc
// @Summary Get low stock items
// @Description Retrieves items where quantity is below or at the low stock threshold
// @Tags analytics
// @Produce json
// @Param global_threshold query int false "Global low stock threshold if item-specific one isn't set (default: 5)"
// @Success 200 {array} domain.Item "List of low stock items"
// @Failure 500 {object} httputil.HTTPError "Internal Server Error"
// @Router /analytics/low-stock [get]
func (h *AnalyticsHandler) GetLowStockItems(c echo.Context) error {
	thresholdStr := c.QueryParam("global_threshold")
	globalThreshold := 5 // Default global threshold
	if thresholdStr != "" {
		parsedThreshold, err := strconv.Atoi(thresholdStr)
		if err == nil && parsedThreshold >= 0 {
			globalThreshold = parsedThreshold
		}
	}

	items, err := h.analyticsService.ListLowStockItems(c.Request().Context(), globalThreshold)
	if err != nil {
		log.Printf("GetLowStockItems: Service error: %v", err)
		return httputil.SendErrorResponse(c, httputil.InternalServerError("Failed to retrieve low stock items."))
	}
	return c.JSON(http.StatusOK, items)
}

// GetMostValuableItems godoc
// @Summary Get most valuable items
// @Description Retrieves the top N items ordered by their total value (quantity * price)
// @Tags analytics
// @Produce json
// @Param limit query int false "Number of items to return (default: 5, max: 50)"
// @Success 200 {array} domain.Item "List of most valuable items"
// @Failure 500 {object} httputil.HTTPError "Internal Server Error"
// @Router /analytics/most-valuable [get]
func (h *AnalyticsHandler) GetMostValuableItems(c echo.Context) error {
	limitStr := c.QueryParam("limit")
	limit := 5 // Default limit
	if limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
    if limit > 50 { // Max limit defined in service, can also be enforced here
        limit = 50
    }


	items, err := h.analyticsService.ListMostValuableItems(c.Request().Context(), limit)
	if err != nil {
		log.Printf("GetMostValuableItems: Service error: %v", err)
		return httputil.SendErrorResponse(c, httputil.InternalServerError("Failed to retrieve most valuable items."))
	}
	return c.JSON(http.StatusOK, items)
}
