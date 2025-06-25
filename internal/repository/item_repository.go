package repository

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"inventory-system/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgItemRepository struct {
	db *pgxpool.Pool
}

// NewPgItemRepository creates a new instance of ItemRepository backed by PostgreSQL.
func NewPgItemRepository(db *pgxpool.Pool) domain.ItemRepository {
	return &pgItemRepository{db: db}
}

// Create inserts a new item into the database.
func (r *pgItemRepository) Create(ctx context.Context, item *domain.Item) (*domain.Item, error) {
	// Generate UUID if not provided (though DB default should handle it)
	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	item.CreatedAt = time.Now()
	item.UpdatedAt = time.Now()

	query := `
        INSERT INTO items (id, sku, name, description, quantity, price, low_stock_threshold, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
        RETURNING id, created_at, updated_at` // Return generated/defaulted fields

	err := r.db.QueryRow(ctx, query,
		item.ID,
		item.SKU,
		item.Name,
		item.Description,
		item.Quantity,
		item.Price,
		item.LowStockThreshold,
		item.CreatedAt,
		item.UpdatedAt,
	).Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt) // Scan the returned values

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			// Check for unique constraint violation (e.g., duplicate SKU)
			if pgErr.Code == "23505" { // PostgreSQL unique_violation error code
				return nil, fmt.Errorf("item with SKU '%s' already exists: %w", item.SKU, err)
			}
		}
		return nil, fmt.Errorf("failed to create item: %w", err)
	}
	return item, nil
}

// GetByID retrieves a single item by its ID.
func (r *pgItemRepository) GetByID(ctx context.Context, id string) (*domain.Item, error) {
	query := `
        SELECT id, sku, name, description, quantity, price, low_stock_threshold, created_at, updated_at
        FROM items
        WHERE id = $1`

	item := &domain.Item{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&item.ID,
		&item.SKU,
		&item.Name,
		&item.Description,
		&item.Quantity,
		&item.Price,
		&item.LowStockThreshold,
		&item.CreatedAt,
		&item.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
    return nil, fmt.Errorf("%w: item with ID '%s'", domain.ErrItemNotFound, id)
}
		return nil, fmt.Errorf("failed to get item by ID:'%s' %w",id, err)
	}
	return item, nil
}

// GetAll retrieves a paginated list of items and the total count.
func (r *pgItemRepository) GetAll(ctx context.Context, page, limit int) ([]*domain.Item, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10 // Default limit
	}
	offset := (page - 1) * limit

	// Query for items
	query := `
        SELECT id, sku, name, description, quantity, price, low_stock_threshold, created_at, updated_at
        FROM items
        ORDER BY created_at DESC
        LIMIT $1 OFFSET $2`

	rows, err := r.db.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get all items: %w", err)
	}
	defer rows.Close()

	var items []*domain.Item
	for rows.Next() {
		item := &domain.Item{}
		err := rows.Scan(
			&item.ID,
			&item.SKU,
			&item.Name,
			&item.Description,
			&item.Quantity,
			&item.Price,
			&item.LowStockThreshold,
			&item.CreatedAt,
			&item.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan item row: %w", err)
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating item rows: %w", err)
	}

	// Query for total count
	var totalItems int
	countQuery := `SELECT COUNT(*) FROM items`
	err = r.db.QueryRow(ctx, countQuery).Scan(&totalItems)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get total item count: %w", err)
	}

	return items, totalItems, nil
}

// Update modifies an existing item in the database.
// It only updates fields that are non-nil in the input 'itemUpdate' (which should be populated from UpdateItemRequest).
func (r *pgItemRepository) Update(ctx context.Context, id string, itemUpdate *domain.Item) (*domain.Item, error) {
	// First, fetch the existing item to see what needs updating
	// and to ensure it exists.
	// This approach is a bit chatty but clear. A more optimized way
	// would be to build a dynamic SQL query.

	existingItem, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err // GetByID already provides a good error message
	}

	// Build the SET clause dynamically
	setClauses := []string{}
	args := []interface{}{}
	argId := 1

	// itemUpdate contains the desired new values.
	// We only update if the new value is different and explicitly provided (for pointers).
	// For this repository method, we assume 'itemUpdate' is the complete desired state for fields to be changed.
	// The service layer should handle constructing this 'itemUpdate' from the UpdateItemRequest.

	if itemUpdate.SKU != "" && itemUpdate.SKU != existingItem.SKU {
		setClauses = append(setClauses, fmt.Sprintf("sku = $%d", argId))
		args = append(args, itemUpdate.SKU)
		argId++
	}
	if itemUpdate.Name != "" && itemUpdate.Name != existingItem.Name {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argId))
		args = append(args, itemUpdate.Name)
		argId++
	}
	// For nullable fields, check if the pointer is non-nil
	if itemUpdate.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argId))
		args = append(args, *itemUpdate.Description)
		argId++
	}
	// Quantity is not a pointer, so we update if it's different.
	// The service should ensure a valid quantity.
	if itemUpdate.Quantity != existingItem.Quantity {
		setClauses = append(setClauses, fmt.Sprintf("quantity = $%d", argId))
		args = append(args, itemUpdate.Quantity)
		argId++
	}
	if itemUpdate.Price != existingItem.Price {
		setClauses = append(setClauses, fmt.Sprintf("price = $%d", argId))
		args = append(args, itemUpdate.Price)
		argId++
	}
	if itemUpdate.LowStockThreshold != nil {
		setClauses = append(setClauses, fmt.Sprintf("low_stock_threshold = $%d", argId))
		args = append(args, *itemUpdate.LowStockThreshold)
		argId++
	}

	if len(setClauses) == 0 {
		log.Println("No fields to update for item ID:", id)
		return existingItem, nil // No changes, return the existing item
	}

	// Always update 'updated_at' (though the trigger should handle this, good to be explicit or if trigger is off)
	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argId))
	args = append(args, time.Now())
	argId++

	args = append(args, id) // For the WHERE clause

	query := fmt.Sprintf(`
        UPDATE items
        SET %s
        WHERE id = $%d
        RETURNING id, sku, name, description, quantity, price, low_stock_threshold, created_at, updated_at`,
		strings.Join(setClauses, ", "), argId)

	updatedItem := &domain.Item{}
	err = r.db.QueryRow(ctx, query, args...).Scan(
		&updatedItem.ID,
		&updatedItem.SKU,
		&updatedItem.Name,
		&updatedItem.Description,
		&updatedItem.Quantity,
		&updatedItem.Price,
		&updatedItem.LowStockThreshold,
		&updatedItem.CreatedAt,
		&updatedItem.UpdatedAt,
	)

	if err != nil {
		var pgErr *pgconn.PgError
if errors.As(err, &pgErr) && pgErr.Code == "23505" {
	return nil, fmt.Errorf("%w: SKU %s", domain.ErrRepositoryDuplicateEntry, itemUpdate.SKU)
}
		return nil, fmt.Errorf("failed to update item: %w", err)
	}
	return updatedItem, nil
}

// Delete removes an item from the database by its ID.
func (r *pgItemRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM items WHERE id = $1`
	commandTag, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete item: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("item with ID '%s' not found for deletion: %w", id, pgx.ErrNoRows) // Or a custom domain.ErrNotFound
	}
	return nil
}

// --- Analytics Methods ---

// GetTotalStockValue calculates the total value of all items in stock.
func (r *pgItemRepository) GetTotalStockValue(ctx context.Context) (float64, error) {
	query := `SELECT COALESCE(SUM(quantity * price), 0) FROM items`
	var totalValue float64
	err := r.db.QueryRow(ctx, query).Scan(&totalValue)
	if err != nil {
		return 0, fmt.Errorf("failed to get total stock value: %w", err)
	}
	return totalValue, nil
}

// GetLowStockItems retrieves items where quantity is at or below their low_stock_threshold.
// If item.low_stock_threshold is NULL, it uses the globalThreshold.
func (r *pgItemRepository) GetLowStockItems(ctx context.Context, globalThreshold int) ([]*domain.Item, error) {
	query := `
        SELECT id, sku, name, description, quantity, price, low_stock_threshold, created_at, updated_at
        FROM items
        WHERE quantity <= COALESCE(low_stock_threshold, $1)
        ORDER BY quantity ASC, name ASC`

	rows, err := r.db.Query(ctx, query, globalThreshold)
	if err != nil {
		return nil, fmt.Errorf("failed to get low stock items: %w", err)
	}
	defer rows.Close()

	var items []*domain.Item
	for rows.Next() {
		item := &domain.Item{}
		err := rows.Scan(
			&item.ID,
			&item.SKU,
			&item.Name,
			&item.Description,
			&item.Quantity,
			&item.Price,
			&item.LowStockThreshold,
			&item.CreatedAt,
			&item.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan low stock item row: %w", err)
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating low stock item rows: %w", err)
	}
	return items, nil
}

// GetMostValuableItems retrieves the top N items by total value (quantity * price).
func (r *pgItemRepository) GetMostValuableItems(ctx context.Context, limit int) ([]*domain.Item, error) {
	if limit <= 0 {
		limit = 5 // Default limit
	}
	query := `
        SELECT id, sku, name, description, quantity, price, low_stock_threshold, created_at, updated_at
        FROM items
        ORDER BY (quantity * price) DESC, name ASC
        LIMIT $1`

	rows, err := r.db.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get most valuable items: %w", err)
	}
	defer rows.Close()

	var items []*domain.Item
	for rows.Next() {
		item := &domain.Item{}
		err := rows.Scan(
			&item.ID,
			&item.SKU,
			&item.Name,
			&item.Description,
			&item.Quantity,
			&item.Price,
			&item.LowStockThreshold,
			&item.CreatedAt,
			&item.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan most valuable item row: %w", err)
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating most valuable item rows: %w", err)
	}
	return items, nil
}
