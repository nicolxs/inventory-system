package domain

import "errors"

// --- Repository Errors ---
// These are errors that repository implementations should wrap and return.
var (
	ErrRepositoryNotFound       = errors.New("repository: resource not found")
	ErrRepositoryDuplicateEntry = errors.New("repository: duplicate entry")
	// Add more specific repository errors if needed, e.g., ErrOptimisticLockFailed
)

// --- Service Errors ---
// These are errors that the service layer returns to the handler/API layer.
// They might wrap repository errors or represent business logic failures.
var (
	ErrItemNotFound      = errors.New("item not found")                    // User-facing, maps from ErrRepositoryNotFound
	ErrInvalidInput      = errors.New("invalid input")                     // General validation error from service
	ErrInvalidItemID     = errors.New("invalid item ID format")            // Specific invalid input
	ErrSKUAlreadyExists  = errors.New("item with this SKU already exists") // Maps from ErrRepositoryDuplicateEntry
	ErrUpdateNoChanges   = errors.New("no changes provided for update")
	ErrInsufficientStock = errors.New("insufficient stock for operation")
	ErrOperationFailed   = errors.New("operation failed") // Generic service operation failure
)
