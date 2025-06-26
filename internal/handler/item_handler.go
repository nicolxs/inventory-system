package handler

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"

	"inventory-system/internal/domain"
	"inventory-system/pkg/httputil" // Our error utility

	"github.com/go-playground/validator/v10" // For request validation
	"github.com/labstack/echo/v4"
)

// Define a regular expression for alphanumeric characters and dashes.
// ^ and $ anchor it to the beginning and end of the string.
// [a-zA-Z0-9-]+ means one or more characters that are letters, numbers, or a dash.
var alphaNumDashRegex = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

// Custom validation function for 'alphanumdash'
func validateAlphaNumDash(fl validator.FieldLevel) bool {
	return alphaNumDashRegex.MatchString(fl.Field().String())
}

// ItemHandler handles HTTP requests for items.
type ItemHandler struct {
	itemService domain.ItemService
	validate    *validator.Validate // Validator instance
}

// NewItemHandler creates a new ItemHandler.
func NewItemHandler(is domain.ItemService) *ItemHandler {
	validate := validator.New() // Initialize a new validator

	// <<<<<<< REGISTER THE CUSTOM VALIDATION HERE >>>>>>>>>
	err := validate.RegisterValidation("alphanumdash", validateAlphaNumDash)
	if err != nil {
		// This is a critical setup error, so we panic.
		// The server will fail to start, which is what we want if validation can't be set up.
		log.Fatalf("Failed to register custom validation: %v", err)
	}

	return &ItemHandler{
		itemService: is,
		validate:    validate, // Use the configured validator
	}
}


// CreateItem godoc
// @Summary Create a new item
// @Description Adds a new item to the inventory
// @Tags items
// @Accept json
// @Produce json
// @Param item body domain.CreateItemRequest true "Item to create"
// @Success 201 {object} domain.Item "Successfully created item"
// @Failure 400 {object} httputil.HTTPError "Bad Request (e.g., invalid input format)"
// @Failure 422 {object} httputil.HTTPError "Unprocessable Entity (validation errors)"
// @Failure 409 {object} httputil.HTTPError "Conflict (e.g., SKU already exists)"
// @Failure 500 {object} httputil.HTTPError "Internal Server Error"
// @Router /items [post]
func (h *ItemHandler) CreateItem(c echo.Context) error {
	var req domain.CreateItemRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("CreateItem: Bind error: %v", err)
		return httputil.SendErrorResponse(c, httputil.BadRequestError("Invalid request payload: "+err.Error()))
	}

	// Validate the request body using struct tags
	if err := h.validate.StructCtx(c.Request().Context(), req); err != nil {
		log.Printf("CreateItem: Validation error: %v", err)
		// Provide more detailed validation errors if desired
		validationErrors := ParseValidationErrors(err)
		return httputil.SendErrorResponse(c, httputil.ValidationError("Input validation failed", validationErrors))
	}

	item, err := h.itemService.CreateItem(c.Request().Context(), &req)
	if err != nil {
		log.Printf("CreateItem: Service error: %v", err)
		if errors.Is(err, domain.ErrSKUAlreadyExists) { // Assuming service.ErrSKUAlreadyExists
			return httputil.SendErrorResponse(c, httputil.ConflictError(err.Error()))
		}
		// Handle other specific domain errors from service if necessary
		return httputil.SendErrorResponse(c, httputil.InternalServerError("Failed to create item."))
	}

	return c.JSON(http.StatusCreated, item)
}

// GetItemByID godoc
// @Summary Get an item by ID
// @Description Retrieves a specific item by its UUID
// @Tags items
// @Produce json
// @Param id path string true "Item ID (UUID)"
// @Success 200 {object} domain.Item "Successfully retrieved item"
// @Failure 400 {object} httputil.HTTPError "Bad Request (invalid ID format)"
// @Failure 404 {object} httputil.HTTPError "Not Found"
// @Failure 500 {object} httputil.HTTPError "Internal Server Error"
// @Router /items/{id} [get]
func (h *ItemHandler) GetItemByID(c echo.Context) error {
	id := c.Param("id")
	// Basic validation for ID format can happen here or in service
	// For UUID, service layer already validates format.

	item, err := h.itemService.GetItemByID(c.Request().Context(), id)
	if err != nil {
		log.Printf("GetItemByID: Service error for ID %s: %v", id, err)
		if errors.Is(err, domain.ErrInvalidItemID) {
			return httputil.SendErrorResponse(c, httputil.BadRequestError(err.Error()))
		}
		if errors.Is(err, domain.ErrItemNotFound) { // Assuming service.ErrItemNotFound
			return httputil.SendErrorResponse(c, httputil.NotFoundError(fmt.Sprintf("Item with ID '%s' not found.", id)))
		}
		return httputil.SendErrorResponse(c, httputil.InternalServerError("Failed to retrieve item."))
	}

	return c.JSON(http.StatusOK, item)
}

// GetItems godoc
// @Summary Get all items (paginated)
// @Description Retrieves a list of items with pagination
// @Tags items
// @Produce json
// @Param page query int false "Page number (default: 1)"
// @Param limit query int false "Items per page (default: 10, max: 100)"
// @Success 200 {object} map[string]interface{} "items":[]domain.Item, "total":int, "page":int, "limit":int "List of items and pagination info"
// @Failure 500 {object} httputil.HTTPError "Internal Server Error"
// @Router /items [get]
func (h *ItemHandler) GetItems(c echo.Context) error {
	pageStr := c.QueryParam("page")
	limitStr := c.QueryParam("limit")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1 // Default page
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 10 // Default limit
	}
	if limit > 100 { // Max limit
		limit = 100
	}

	items, total, err := h.itemService.GetItems(c.Request().Context(), page, limit)
	if err != nil {
		log.Printf("GetItems: Service error: %v", err)
		return httputil.SendErrorResponse(c, httputil.InternalServerError("Failed to retrieve items."))
	}

	response := struct {
		Items []*domain.Item `json:"items"`
		Total int            `json:"total"`
		Page  int            `json:"page"`
		Limit int            `json:"limit"`
	}{
		Items: items,
		Total: total,
		Page:  page,
		Limit: limit,
	}

	return c.JSON(http.StatusOK, response)
}

// UpdateItem godoc
// @Summary Update an existing item
// @Description Updates specified fields of an existing item by its UUID
// @Tags items
// @Accept json
// @Produce json
// @Param id path string true "Item ID (UUID)"
// @Param item body domain.UpdateItemRequest true "Fields to update"
// @Success 200 {object} domain.Item "Successfully updated item"
// @Failure 400 {object} httputil.HTTPError "Bad Request (e.g., invalid ID or input format)"
// @Failure 404 {object} httputil.HTTPError "Not Found"
// @Failure 409 {object} httputil.HTTPError "Conflict (e.g., SKU already exists after update)"
// @Failure 422 {object} httputil.HTTPError "Unprocessable Entity (validation errors)"
// @Failure 500 {object} httputil.HTTPError "Internal Server Error"
// @Router /items/{id} [put]
func (h *ItemHandler) UpdateItem(c echo.Context) error {
	id := c.Param("id")
	// ID format validation done by service

	var req domain.UpdateItemRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("UpdateItem: Bind error for ID %s: %v", id, err)
		return httputil.SendErrorResponse(c, httputil.BadRequestError("Invalid request payload: "+err.Error()))
	}

	// Validate the request body
	if err := h.validate.StructCtx(c.Request().Context(), req); err != nil {
		log.Printf("UpdateItem: Validation error for ID %s: %v", id, err)
		validationErrors := ParseValidationErrors(err)
		return httputil.SendErrorResponse(c, httputil.ValidationError("Input validation failed", validationErrors))
	}

	item, err := h.itemService.UpdateItem(c.Request().Context(), id, &req)
	if err != nil {
		log.Printf("UpdateItem: Service error for ID %s: %v", id, err)
		if errors.Is(err, domain.ErrInvalidItemID) {
			return httputil.SendErrorResponse(c, httputil.BadRequestError(err.Error()))
		}
		if errors.Is(err, domain.ErrItemNotFound) {
			return httputil.SendErrorResponse(c, httputil.NotFoundError(fmt.Sprintf("Item with ID '%s' not found for update.", id)))
		}
		if errors.Is(err, domain.ErrSKUAlreadyExists) {
			return httputil.SendErrorResponse(c, httputil.ConflictError(err.Error()))
		}
		// if errors.Is(err, domain.ErrUpdateNoChanges) { // If service returns this
		// 	return httputil.SendErrorResponse(c, httputil.BadRequestError("No changes provided in the update request."))
		// }
		return httputil.SendErrorResponse(c, httputil.InternalServerError("Failed to update item."))
	}

	return c.JSON(http.StatusOK, item)
}

// DeleteItem godoc
// @Summary Delete an item by ID
// @Description Deletes a specific item by its UUID
// @Tags items
// @Produce json
// @Param id path string true "Item ID (UUID)"
// @Success 204 "Successfully deleted item (No Content)"
// @Failure 400 {object} httputil.HTTPError "Bad Request (invalid ID format)"
// @Failure 404 {object} httputil.HTTPError "Not Found"
// @Failure 500 {object} httputil.HTTPError "Internal Server Error"
// @Router /items/{id} [delete]
func (h *ItemHandler) DeleteItem(c echo.Context) error {
	id := c.Param("id")
	// ID format validation done by service

	err := h.itemService.DeleteItem(c.Request().Context(), id)
	if err != nil {
		log.Printf("DeleteItem: Service error for ID %s: %v", id, err)
		if errors.Is(err, domain.ErrInvalidItemID) {
			return httputil.SendErrorResponse(c, httputil.BadRequestError(err.Error()))
		}
		if errors.Is(err, domain.ErrItemNotFound) {
			return httputil.SendErrorResponse(c, httputil.NotFoundError(fmt.Sprintf("Item with ID '%s' not found for deletion.", id)))
		}
		return httputil.SendErrorResponse(c, httputil.InternalServerError("Failed to delete item."))
	}

	return c.NoContent(http.StatusNoContent)
}


// ParseValidationErrors is a helper to convert validator.ValidationErrors into a map.
func ParseValidationErrors(err error) map[string]string {
	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		out := make(map[string]string, len(ve))
		for _, fe := range ve {
			// Use fe.Field() for field name, fe.Tag() for the rule failed
			out[fe.Field()] = fmt.Sprintf("Failed validation on rule '%s'", fe.Tag())
			// You can customize messages further based on fe.Tag() or fe.Param()
			// e.g., if fe.Tag() == "required", msg = "This field is required"
		}
		return out
	}
	// If it's not validator.ValidationErrors, return a generic message or nil
	return map[string]string{"error": "Invalid input data"}
}
