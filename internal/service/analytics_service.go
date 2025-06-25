package service

import (
	"context"
	"fmt"

	"inventory-system/internal/domain"
)

type analyticsService struct {
	itemRepo domain.ItemRepository // Assuming ItemRepository also handles analytics queries
}

// NewAnalyticsService creates a new AnalyticsService.
func NewAnalyticsService(itemRepo domain.ItemRepository) domain.AnalyticsService {
	return &analyticsService{itemRepo: itemRepo}
}

// CalculateTotalStockValue calculates the total stock value.
func (s *analyticsService) CalculateTotalStockValue(ctx context.Context) (float64, error) {
	value, err := s.itemRepo.GetTotalStockValue(ctx)
	if err != nil {
		return 0, fmt.Errorf("service: failed to calculate total stock value: %w", err)
	}
	return value, nil
}

// ListLowStockItems lists items that are low in stock.
func (s *analyticsService) ListLowStockItems(ctx context.Context, globalThreshold int) ([]*domain.Item, error) {
	if globalThreshold < 0 {
		globalThreshold = 5 // Default global threshold if not sensible
	}
	items, err := s.itemRepo.GetLowStockItems(ctx, globalThreshold)
	if err != nil {
		return nil, fmt.Errorf("service: failed to list low stock items: %w", err)
	}
	return items, nil
}

// ListMostValuableItems lists the top N most valuable items.
func (s *analyticsService) ListMostValuableItems(ctx context.Context, limit int) ([]*domain.Item, error) {
	if limit <= 0 {
		limit = 5 // Default limit
	}
	if limit > 50 {
		limit = 50 // Max limit for this query
	}
	items, err := s.itemRepo.GetMostValuableItems(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("service: failed to list most valuable items: %w", err)
	}
	return items, nil
}
