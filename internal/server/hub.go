package server

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/Elive/synthwave-player/internal/config"
	"github.com/Elive/synthwave-player/internal/models"
	"github.com/coder/websocket"
)

type Hub struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]struct{}
	version string
	hash    string
}

func NewHub(sourceHash string) *Hub {
	return &Hub{
		clients: make(map[*websocket.Conn]struct{}),
		version: config.AppVersion,
		hash:    sourceHash,
	}
}

func (h *Hub) Add(c *websocket.Conn) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) Remove(c *websocket.Conn) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

func (h *Hub) broadcast(v interface{}) {
	data, _ := json.Marshal(v)
	h.mu.Lock()
	defer h.mu.Unlock()
	ctx := context.Background()
	for c := range h.clients {
		_ = c.Write(ctx, websocket.MessageText, data)
	}
}

func (h *Hub) BroadcastStatusUpdate(st models.LoadingStatus) {
	h.broadcast(map[string]interface{}{"event": "status_update", "status": st})
}

func (h *Hub) SendStatusUpdate(c *websocket.Conn, st models.LoadingStatus) {
	h.send(c, map[string]interface{}{"event": "status_update", "status": st})
}
func (h *Hub) BroadcastLibrary() { h.broadcast(map[string]string{"event": "library_updated"}) }
func (h *Hub) BroadcastConfig()  { h.broadcast(map[string]string{"event": "config_updated"}) }

func (h *Hub) BroadcastLyrics(songID, lyrics string) {
	h.broadcast(map[string]interface{}{"event": "lyrics_update", "song_id": songID, "lyrics": lyrics})
}

func (h *Hub) SendVersion(c *websocket.Conn) {
	h.send(c, map[string]interface{}{
		"event": "version", "source_hash": h.hash, "version": h.version,
	})
}

func (h *Hub) send(c *websocket.Conn, v interface{}) {
	data, _ := json.Marshal(v)
	_ = c.Write(context.Background(), websocket.MessageText, data)
}
