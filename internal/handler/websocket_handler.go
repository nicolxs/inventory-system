package handler

import (
	"log"

	"inventory-system/internal/realtime" // Our WebSocket hub

	"github.com/labstack/echo/v4"
)

// WebSocketHandler handles WebSocket connection upgrades.
type WebSocketHandler struct {
	hub *realtime.Hub
}

// NewWebSocketHandler creates a new WebSocketHandler.
func NewWebSocketHandler(hub *realtime.Hub) *WebSocketHandler {
	return &WebSocketHandler{hub: hub}
}

// HandleConnections upgrades HTTP requests to WebSocket connections.
// It uses the ServeWsUpgrade helper from the realtime package.
// @Summary Establish WebSocket connection for stock updates
// @Description Upgrades HTTP GET request to a WebSocket connection.
// @Tags websockets
// @Router /ws/stock-updates [get]
func (h *WebSocketHandler) HandleConnections(c echo.Context) error {
	log.Printf("Incoming WebSocket connection request from: %s", c.Request().RemoteAddr)
	// The ServeWsUpgrade function from realtime package handles the upgrade
	// and client registration with the hub.
	realtime.ServeWsUpgrade(h.hub, c.Response().Writer, c.Request())
	// ServeWsUpgrade doesn't return an error in a way Echo expects for its chain,
	// as it takes over the connection. If it fails, it logs and writes an HTTP error itself.
	// So, we typically return nil here to Echo, indicating the handler has managed the response.
	return nil
}
