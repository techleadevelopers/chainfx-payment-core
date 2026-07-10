package mobile

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(_ *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
}

// wsHub manages WebSocket clients and broadcasts.
type wsHub struct {
	mu      sync.RWMutex
	clients map[string]map[*wsClient]bool // topic → clients
}

type wsClient struct {
	conn  *websocket.Conn
	send  chan []byte
	topic string
	uid   string
}

func newWsHub() *wsHub {
	return &wsHub{clients: make(map[string]map[*wsClient]bool)}
}

func (h *wsHub) run() {
	// Heartbeat every 30s to keep connections alive
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		h.broadcast("price", []byte(`{"type":"ping"}`))
	}
}

func (h *wsHub) register(topic string, c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[topic] == nil {
		h.clients[topic] = make(map[*wsClient]bool)
	}
	h.clients[topic][c] = true
}

func (h *wsHub) unregister(topic string, c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[topic] != nil {
		delete(h.clients[topic], c)
	}
}

func (h *wsHub) broadcast(topic string, msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients[topic] {
		select {
		case c.send <- msg:
		default:
			close(c.send)
		}
	}
}

// BroadcastPrice is called by the price worker to push live prices.
func (s *Server) BroadcastPrice(priceBRL float64) {
	msg, _ := marshalJSON(map[string]any{"type": "price", "usdt_brl": priceBRL, "ts": time.Now().Unix()})
	s.hub.broadcast("price", msg)
}

// BroadcastOrderUpdate is called when an order status changes.
func (s *Server) BroadcastOrderUpdate(orderID, status string) {
	msg, _ := marshalJSON(map[string]any{"type": "order_update", "order_id": orderID, "status": status, "ts": time.Now().Unix()})
	s.hub.broadcast("orders", msg)
}

func serveWS(h *wsHub, topic, uid string, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("ws upgrade failed", "err", err)
		return
	}
	c := &wsClient{conn: conn, send: make(chan []byte, 64), topic: topic, uid: uid}
	h.register(topic, c)
	defer func() {
		h.unregister(topic, c)
		conn.Close()
	}()

	// Writer goroutine
	go func() {
		for msg := range c.send {
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		}
	}()

	// Send welcome message
	welcome, _ := marshalJSON(map[string]any{"type": "connected", "topic": topic})
	c.send <- welcome

	// Reader loop (keep alive / handle pong)
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			return
		}
	}
}

// handleWSOrders — WS /api/mobile/ws/orders
func (s *Server) handleWSOrders(w http.ResponseWriter, r *http.Request) {
	uid := ""
	auth := r.Header.Get("Authorization")
	if auth != "" {
		if claims, err := verifyToken(s.mcfg.JWTSecret, auth[7:]); err == nil {
			uid = claims.Sub
		}
	}
	serveWS(s.hub, "orders", uid, w, r)
}

// handleWSPrice — WS /api/mobile/ws/price
func (s *Server) handleWSPrice(w http.ResponseWriter, r *http.Request) {
	serveWS(s.hub, "price", "", w, r)
}

// handleWSNotifications — WS /api/mobile/ws/notifications
func (s *Server) handleWSNotifications(w http.ResponseWriter, r *http.Request) {
	uid := ""
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 {
		if claims, err := verifyToken(s.mcfg.JWTSecret, auth[7:]); err == nil {
			uid = claims.Sub
		}
	}
	serveWS(s.hub, "notifications:"+uid, uid, w, r)
}
