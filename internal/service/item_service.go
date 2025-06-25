package service

import (
	"context"
	"errors" // For domain-specific errors if you define them
	"fmt"
	"log"
	// "time" // Not directly needed here anymore unless for specific logic

	"inventory-system/internal/domain"
	"inventory-system/internal/realtime" // For WebSocket Hub

	"github.com/google/uuid"
)

// Predefined errors (optional, but good practice for domain errors)
var (
	ErrItemNotFound      = errors.New("item not found")
	ErrInvalidItemID     = errors.New("invalid item ID format")
	ErrSKUAlreadyExists  = errors.New("item with this SKU already exists")
	ErrUpdateNoChanges   = errors.New("no changes provided for update")
)


type itemService struct {
	repo domain.ItemRepository
	hub  *realtime.Hub // WebSocket hub for real-time updates
}

// NewItemService creates a new ItemService.
func NewItemService(repo domain.ItemRepository, hub *realtime.Hub) domain.ItemService {
	return &itemService{
		repo: repo,
		hub:  hub,
	}
}

// CreateItem handles the business logic for creating a new item.
func (s *itemService) CreateItem(ctx context.Context, req *domain.CreateItemRequest) (*domain.Item, error) {
	// Validation (e.g., using struct tags) should ideally occur in the handler layer
	// before reaching the service. If basic checks are done here, ensure they are minimal.

	newItem := &domain.Item{
		ID:                uuid.NewString(), // Service layer generates ID
		SKU:               req.SKU,
		Name:              req.Name,
		Description:       req.Description,       // Assumes Description is *string
		Quantity:          req.Quantity,
		Price:             req.Price,
		LowStockThreshold: req.LowStockThreshold, // Assumes LowStockThreshold is *int
		// CreatedAt and UpdatedAt are set by the repository or database.
	}

	createdItem, err := s.repo.Create(ctx, newItem)
	if err != nil {
		// Check if the error from repository indicates a duplicate SKU
		// This depends on how the repository wraps the pgconn.PgError
		if errors.Is(err, domain.ErrRepositoryDuplicateEntry) { // Assuming repo wraps pgErr.Code == "23505"
		    return nil, fmt.Errorf("%w: SKU %s", ErrSKUAlreadyExists, req.SKU)
		}
		return nil, fmt.Errorf("service: failed to create item: %w", err)
	}

	// Example: Broadcast an event if necessary (e.g., "NEW_ITEM_ADDED")
	// This depends on frontend requirements. For now, only stock quantity changes are broadcasted.

	return createdItem, nil
}

// GetItemByID retrieves an item by its ID.
func (s *itemService) GetItemByID(ctx context.Context, id string) (*domain.Item, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidItemID, id)
	}

	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrRepositoryNotFound) { // Assuming repo wraps pgx.ErrNoRows
			return nil, fmt.Errorf("%w: ID %s", ErrItemNotFound, id)
		}
		return nil, fmt.Errorf("service: failed to get item by ID '%s': %w", id, err)
	}
	return item, nil
}

// GetItems retrieves a paginated list of items.
func (s *itemService) GetItems(ctx context.Context, page, limit int) ([]*domain.Item, int, error) {
	if page <= 0 {
		page = 1
	}
	// Apply reasonable defaults and maximums for pagination
	if limit <= 0 {
		limit = 10
	} else if limit > 100 {
		limit = 100
	}

	items, total, err := s.repo.GetAll(ctx, page, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("service: failed to get items: %w", err)
	}
	return items, total, nil
}

// UpdateItem handles the business logic for updating an item.
func (s *itemService) UpdateItem(ctx context.Context, id string, req *domain.UpdateItemRequest) (*domain.Item, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, fmt.Errorf("%w: %s for update", ErrInvalidItemID, id)
	}

	// Fetch the existing item. This is crucial for:
	// 1. Ensuring the item exists.
	// 2. Getting the original quantity for WebSocket comparison.
	// 3. Providing a base for applying partial updates.
	existingItem, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrRepositoryNotFound) {
			return nil, fmt.Errorf("%w: ID %s for update", ErrItemNotFound, id)
		}
		return nil, fmt.Errorf("service: error fetching item for update (ID: %s): %w", id, err)
	}
	originalQuantity := existingItem.Quantity

	// Construct the item object that will be passed to the repository's Update method.
	// This object should only have fields set if they are explicitly provided in the request
	// and are different from the existing values.
	// The repository's Update method is expected to handle a *domain.Item where non-nil/non-zero
	// fields (for pointers/value types respectively if that's its contract) indicate an update.
	// Or, more robustly, the repository takes a map[string]interface{} or a dedicated update struct.

	// Let's refine how `itemToUpdate` is constructed to align with the dynamic SQL
	// in the repository. The repo's Update method expects a full *domain.Item,
	// and it will check which fields are different from the existing one (or are non-zero/non-nil).
	// A better approach for the repo would be to accept a struct that clearly signals intent,
	// like map[string]interface{} or a dedicated struct with pointers for all updatable fields.
	// Given our current repo, we need to carefully construct `itemToUpdate`.

	itemForUpdate := &domain.Item{
		// ID is not part of the payload, it's the identifier for the WHERE clause.
		// Start with existing values, then selectively override.
		// This is a bit redundant if the repo already fetches, but good for clarity here.
		SKU:               existingItem.SKU,
		Name:              existingItem.Name,
		Description:       existingItem.Description,
		Quantity:          existingItem.Quantity,
		Price:             existingItem.Price,
		LowStockThreshold: existingItem.LowStockThreshold,
		// Timestamps (CreatedAt, UpdatedAt) are handled by repo/DB.
	}
	
	madeChange := false // Track if any field was actually changed by the request

	if req.SKU != nil {
		if *req.SKU != itemForUpdate.SKU {
			itemForUpdate.SKU = *req.SKU
			madeChange = true
		}
	}
	if req.Name != nil {
		if *req.Name != itemForUpdate.Name {
			itemForUpdate.Name = *req.Name
			madeChange = true
		}
	}
	// For Description (*string), if req.Description is nil, it means "no change".
	// If req.Description points to an empty string, it means "set to empty".
	// If req.Description points to a non-empty string, it means "set to this string".
	if req.Description != nil {
        // Check if the current description is also nil or if values differ
        if itemForUpdate.Description == nil || *req.Description != *itemForUpdate.Description {
            itemForUpdate.Description = req.Description
            madeChange = true
        }
	}
	if req.Quantity != nil {
		if *req.Quantity != itemForUpdate.Quantity {
			itemForUpdate.Quantity = *req.Quantity
			madeChange = true
		}
	}
	if req.Price != nil {
		if *req.Price != itemForUpdate.Price {
			itemForUpdate.Price = *req.Price
			madeChange = true
		}
	}
	if req.LowStockThreshold != nil {
        if itemForUpdate.LowStockThreshold == nil || *req.LowStockThreshold != *itemForUpdate.LowStockThreshold {
            itemForUpdate.LowStockThreshold = req.LowStockThreshold
            madeChange = true
        }
	}

	if !madeChange {
		log.Printf("Service: No actual changes provided for item ID %s. Returning existing item.", id)
		return existingItem, nil // Or return `ErrUpdateNoChanges`
	}

	// Now, `itemForUpdate` contains the full desired state after applying changes.
	// The repository's Update method will compare this with the current DB state (or re-fetch)
	// to build the SET clauses.
	updatedItem, err := s.repo.Update(ctx, id, itemForUpdate)
	if err != nil {
        if errors.Is(err, domain.ErrRepositoryDuplicateEntry) { // SKU conflict during update
		    return nil, fmt.Errorf("%w: SKU %s", ErrSKUAlreadyExists, itemForUpdate.SKU)
		}
		return nil, fmt.Errorf("service: failed to update item ID '%s': %w", id, err)
	}

	// If quantity changed, broadcast the update via WebSocket
	if s.hub != nil && updatedItem.Quantity != originalQuantity {
		log.Printf("Service: Quantity changed for item %s (SKU: %s) from %d to %d. Broadcasting.",
			updatedItem.ID, updatedItem.SKU, originalQuantity, updatedItem.Quantity)

		payload := domain.StockUpdatePayload{
			ID:          updatedItem.ID,
			SKU:         updatedItem.SKU,
			NewQuantity: updatedItem.Quantity,
		}
		s.hub.BroadcastStockUpdate(payload)
	}

	return updatedItem, nil
}

// DeleteItem handles the business logic for deleting an item.
func (s *itemService) DeleteItem(ctx context.Context, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("%w: %s for deletion", ErrInvalidItemID, id)
	}

	// Optional: Check existence first to provide a clearer "not found" vs. "delete failed"
	// For simplicity, we let the repository handle the "not found" on delete.
	err := s.repo.Delete(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrRepositoryNotFound) {
			return fmt.Errorf("%w: ID %s for deletion", ErrItemNotFound, id)
		}
		return fmt.Errorf("service: failed to delete item ID '%s': %w", id, err)
	}

	// Optionally, broadcast "ITEM_DELETED" event via WebSocket
	// if s.hub != nil {
	//  s.hub.BroadcastItemDeleted(id, existingItem.SKU) // Example
	// }

	return nil
}
