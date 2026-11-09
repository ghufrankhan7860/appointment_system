package services

import "appointment_system/internal/apps/appointment/models"

// CallPeer is one open signaling connection. Send must not block: a peer
// that cannot keep up is closed instead.
type CallPeer interface {
	PeerID() string
	Send(signal models.CallSignal) error
	Close()
}

// CallHub delivers signaling messages to the connections open in this
// process. It is implemented outside this package (adapters/callhub) and must
// be safe for concurrent use. It holds no rules: who may receive what is
// decided by the services, which pass the recipients' peer ids.
type CallHub interface {
	// Join adds peer to the room and returns the connection it replaced for
	// the same peer id, or nil.
	Join(roomID uint64, peer CallPeer) CallPeer
	// Leave removes peer and reports whether it was still the room's
	// connection for its peer id.
	Leave(roomID uint64, peer CallPeer) bool
	Send(roomID uint64, peerID string, signal models.CallSignal)
	Broadcast(roomID uint64, peerIDs []string, signal models.CallSignal)
	// ClosePeer sends signal to one peer and closes its connection.
	ClosePeer(roomID uint64, peerID string, signal models.CallSignal)
	// CloseRoom sends signal to every peer of the room and closes them.
	CloseRoom(roomID uint64, signal models.CallSignal)
	CloseAll()
}
