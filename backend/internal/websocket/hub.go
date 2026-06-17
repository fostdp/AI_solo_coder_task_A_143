package websocket

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"crossbow-simulation/backend/internal/model"
	"crossbow-simulation/backend/config"

	"github.com/gorilla/websocket"
)

type Client struct {
	Conn       *websocket.Conn
	Send       chan []byte
	CrossbowID string
}

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	Register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
	crossbowClients map[string]map[*Client]bool
	pingInterval time.Duration
	pongTimeout  time.Duration
	writeWait    time.Duration
}

func NewHub() *Hub {
	cfg := config.AppConfig.WebSocket
	return &Hub{
		clients:         make(map[*Client]bool),
		broadcast:       make(chan []byte, 256),
		register:        make(chan *Client),
		unregister:      make(chan *Client),
		crossbowClients: make(map[string]map[*Client]bool),
		pingInterval:    time.Duration(cfg.PingInterval) * time.Second,
		pongTimeout:     time.Duration(cfg.PongTimeout) * time.Second,
		writeWait:       time.Duration(cfg.WriteWait) * time.Second,
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.mu.Lock()
			h.clients[client] = true
			if _, ok := h.crossbowClients[client.CrossbowID]; !ok {
				h.crossbowClients[client.CrossbowID] = make(map[*Client]bool)
			}
			h.crossbowClients[client.CrossbowID][client] = true
			h.mu.Unlock()
			log.Printf("Client registered for crossbow: %s", client.CrossbowID)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				if clients, ok := h.crossbowClients[client.CrossbowID]; ok {
					delete(clients, client)
					if len(clients) == 0 {
						delete(h.crossbowClients, client.CrossbowID)
					}
				}
				close(client.Send)
			}
			h.mu.Unlock()
			log.Printf("Client unregistered for crossbow: %s", client.CrossbowID)

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) BroadcastSensorData(crossbowID string, data *model.SensorData) {
	msg := model.WSMessage{
		Type:      "sensor_data",
		Payload:   data,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	h.broadcastToCrossbow(crossbowID, msg)
}

func (h *Hub) BroadcastDynamicsState(crossbowID string, state *model.DynamicsState) {
	msg := model.WSMessage{
		Type:      "dynamics_state",
		Payload:   state,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	h.broadcastToCrossbow(crossbowID, msg)
}

func (h *Hub) BroadcastTrajectory(crossbowID string, trajectory *model.ArrowTrajectory) {
	msg := model.WSMessage{
		Type:      "trajectory",
		Payload:   trajectory,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	h.broadcastToCrossbow(crossbowID, msg)
}

func (h *Hub) BroadcastAlert(crossbowID string, alert *model.Alert) {
	msg := model.WSMessage{
		Type:      "alert",
		Payload:   alert,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	h.broadcastToCrossbow(crossbowID, msg)
}

func (h *Hub) BroadcastRLUpdate(crossbowID string, status *model.RLStatus) {
	msg := model.WSMessage{
		Type:      "rl_update",
		Payload:   status,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	h.broadcastToCrossbow(crossbowID, msg)
}

func (h *Hub) BroadcastStatus(crossbowID string, status map[string]interface{}) {
	msg := model.WSMessage{
		Type:      "status",
		Payload:   status,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	h.broadcastToCrossbow(crossbowID, msg)
}

func (h *Hub) broadcastToCrossbow(crossbowID string, msg model.WSMessage) {
	jsonData, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling WS message: %v", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if clients, ok := h.crossbowClients[crossbowID]; ok {
		for client := range clients {
			select {
			case client.Send <- jsonData:
			default:
				close(client.Send)
				delete(clients, client)
				if len(clients) == 0 {
					delete(h.crossbowClients, crossbowID)
				}
			}
		}
	}
}

func (c *Client) ReadPump(hub *Hub) {
	defer func() {
		hub.unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(512)
	c.Conn.SetReadDeadline(time.Now().Add(hub.pongTimeout))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(hub.pongTimeout))
		return nil
	})

	for {
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}
	}
}

func (c *Client) WritePump(hub *Hub) {
	ticker := time.NewTicker(hub.pingInterval)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(hub.writeWait))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(hub.writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
