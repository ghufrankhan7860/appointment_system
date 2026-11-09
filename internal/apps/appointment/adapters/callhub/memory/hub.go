// Package memory keeps call signaling connections in this process's memory.
// It is enough while each environment runs one API container; running several
// needs a hub that relays between them (for example Redis pub/sub).
package memory

import (
	"sync"

	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
)

type Hub struct {
	mu    sync.Mutex
	rooms map[uint64]map[string]services.CallPeer
}

func New() *Hub {
	return &Hub{rooms: make(map[uint64]map[string]services.CallPeer)}
}

func (h *Hub) Join(roomID uint64, peer services.CallPeer) services.CallPeer {
	h.mu.Lock()
	defer h.mu.Unlock()

	peers, ok := h.rooms[roomID]
	if !ok {
		peers = make(map[string]services.CallPeer)
		h.rooms[roomID] = peers
	}
	replaced := peers[peer.PeerID()]
	peers[peer.PeerID()] = peer

	return replaced
}

func (h *Hub) Leave(roomID uint64, peer services.CallPeer) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	peers := h.rooms[roomID]
	if peers[peer.PeerID()] != peer {
		return false
	}
	delete(peers, peer.PeerID())
	if len(peers) == 0 {
		delete(h.rooms, roomID)
	}

	return true
}

func (h *Hub) Send(roomID uint64, peerID string, signal models.CallSignal) {
	if peer := h.peer(roomID, peerID); peer != nil {
		_ = peer.Send(signal)
	}
}

func (h *Hub) Broadcast(roomID uint64, peerIDs []string, signal models.CallSignal) {
	for _, peerID := range peerIDs {
		h.Send(roomID, peerID, signal)
	}
}

func (h *Hub) ClosePeer(roomID uint64, peerID string, signal models.CallSignal) {
	h.mu.Lock()
	peer := h.rooms[roomID][peerID]
	if peer != nil {
		delete(h.rooms[roomID], peerID)
	}
	h.mu.Unlock()

	if peer != nil {
		_ = peer.Send(signal)
		peer.Close()
	}
}

func (h *Hub) CloseRoom(roomID uint64, signal models.CallSignal) {
	h.mu.Lock()
	peers := h.rooms[roomID]
	delete(h.rooms, roomID)
	h.mu.Unlock()

	for _, peer := range peers {
		_ = peer.Send(signal)
		peer.Close()
	}
}

func (h *Hub) CloseAll() {
	h.mu.Lock()
	rooms := h.rooms
	h.rooms = make(map[uint64]map[string]services.CallPeer)
	h.mu.Unlock()

	for _, peers := range rooms {
		for _, peer := range peers {
			peer.Close()
		}
	}
}

func (h *Hub) peer(roomID uint64, peerID string) services.CallPeer {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.rooms[roomID][peerID]
}
