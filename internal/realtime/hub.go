package realtime

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"inventory-system/internal/domain"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

// Client represents a single WebSocket client connection.
type Client struct {
	hub  *Hub            // Reference to the hub.
	conn *websocket.Conn // The WebSocket connection.
	send chan []byte     // Buffered channel of outbound messages. (These will be JSON bytes)
}

// Hub maintains the set of active clients and broadcasts messages to them.
type Hub struct {
	clients    map[*Client]bool // Registered clients.
	broadcast  chan []byte      // Inbound messages from the application (expecting JSON bytes).
	register   chan *Client     // Register requests from clients.
	unregister chan *Client     // Unregister requests from clients.
	mu         sync.RWMutex     // For concurrent access to clients map
}

// NewHub creates a new Hub instance.
func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte, 256), // Buffered channel
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

// Run starts the hub's event loop.
// It must be run in a separate goroutine.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			log.Printf("Client registered: %s, total clients: %d", client.conn.RemoteAddr(), len(h.clients))
			h.mu.Unlock()
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send) // Important to close the send channel
				log.Printf("Client unregistered: %s, total clients: %d", client.conn.RemoteAddr(), len(h.clients))
			}
			h.mu.Unlock()
		case message := <-h.broadcast: // message here is expected to be JSON []byte
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default: // Don't block if client's send buffer is full
					log.Printf("Client send buffer full or closed for %s, unregistering.", client.conn.RemoteAddr())
					// Schedule unregistration to avoid deadlock if unregister channel is also blocked
					// Or simply close the client's send channel and let writePump handle cleanup.
					// For simplicity here, we'll let writePump detect the closed channel.
					// A more robust solution might involve a separate goroutine for timed unregistration.
					// For now, we'll just log and potentially drop the message for this client.
					// If we delete from h.clients here, we need write lock, and to be careful with iteration.
					// It's safer to let the client's own pumps handle their demise.
				}
			}
			h.mu.RUnlock()
		}
	}
}

// BroadcastJSONMessage sends a pre-marshalled JSON message to all connected clients.
// This method is safe for concurrent use.
func (h *Hub) BroadcastJSONMessage(jsonMessage []byte) {
	// Non-blocking send to broadcast channel
	select {
	case h.broadcast <- jsonMessage:
	default:
		log.Println("Hub broadcast channel is full. Message dropped.")
	}
}

// BroadcastStockUpdate marshals and broadcasts a stock update message.
func (h *Hub) BroadcastStockUpdate(payload domain.StockUpdatePayload) {
	wsMessage := domain.WebSocketMessage{
		Type:    domain.StockUpdateMessageType,
		Payload: payload,
	}

	jsonBytes, err := json.Marshal(wsMessage) // <<<<<<<<<<< CORRECT JSON MARSHALING
	if err != nil {
		log.Printf("Error marshalling stock update WebSocket message: %v", err)
		return
	}
	h.BroadcastJSONMessage(jsonBytes)
}

// writePump pumps messages from the hub to the WebSocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close() // Ensure connection is closed on exit
		log.Printf("writePump for client %s stopped.", c.conn.RemoteAddr())
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				// The hub closed the channel (client was unregistered).
				log.Printf("Hub closed send channel for client %s. Sending close message.", c.conn.RemoteAddr())
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			err := c.conn.WriteMessage(websocket.TextMessage, message) // Send as TextMessage, as it's JSON
			if err != nil {
				log.Printf("Error writing message to client %s: %v", c.conn.RemoteAddr(), err)
				// Don't unregister here directly, let readPump or Run handle it
				// to avoid race conditions with the hub's client map.
				// The connection will likely be detected as broken by readPump or the next write.
				return // Exit writePump
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("Error sending ping to client %s: %v", c.conn.RemoteAddr(), err)
				return // Assume connection is dead
			}
		}
	}
}

// readPump pumps messages from the WebSocket connection to the hub (if needed).
// For this app, it mainly handles ping/pong and connection closure.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c // Signal hub to unregister this client
		c.conn.Close()        // Close the WebSocket connection
		log.Printf("readPump for client %s stopped.", c.conn.RemoteAddr())
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		// Clients are not expected to send application messages, only control frames (pong).
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Unexpected WebSocket close error for client %s: %v", c.conn.RemoteAddr(), err)
			} else if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("Client %s closed connection normally.", c.conn.RemoteAddr())
			} else {
				log.Printf("WebSocket read error for client %s (likely closed or timeout): %v", c.conn.RemoteAddr(), err)
			}
			break // Exit loop, defer will unregister and close
		}
		// If we were expecting messages from the client, we'd process them here.
	}
}

// ServeWsUpgrade upgrades the HTTP connection to a WebSocket connection and registers the client.
// This function will be called by the WebSocket handler.
// (This is a helper that will be used by handler/websocket_handler.go)
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for development.
		// For production, you should implement proper origin checking.
		// Example: return r.Header.Get("Origin") == "http://localhost:3000"
		return true
	},
}

func ServeWsUpgrade(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade to WebSocket for %s: %v", r.RemoteAddr, err)
		return
	}
	log.Printf("WebSocket connection upgraded for: %s", conn.RemoteAddr())

	client := &Client{
		hub:  hub,
		conn: conn,
		send: make(chan []byte, 256), // Buffered channel for client's outgoing messages
	}
	client.hub.register <- client // Register the new client with the hub

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.writePump()
	go client.readPump()

	log.Printf("Client %s pumps started.", conn.RemoteAddr())
}
