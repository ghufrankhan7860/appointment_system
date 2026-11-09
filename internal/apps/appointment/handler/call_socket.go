package handler

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/dtos/call"
	"appointment_system/internal/apps/appointment/models"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// callSocketUpgrader keeps gorilla's default origin check: a browser may only
// open the socket from the site that serves the API. Upgrade also clears the
// read and write deadlines the HTTP server set on the connection.
var callSocketUpgrader = websocket.Upgrader{}

var (
	errCallSocketClosed = errors.New("call socket is closed")
	errCallSocketBehind = errors.New("call socket fell behind")
)

// Socket godoc
// @Summary Open the call signaling WebSocket
// @Description Upgrades to a WebSocket for the call the token was issued for (wsToken from a join response, valid 15 minutes). Messages are JSON {"type": ...}. The server sends welcome (your peerId and the peers already in), peer-joined, peer-left, lobby, offer/answer/ice (with from), media, host, ended and error. Clients send offer, answer and ice (with to), media {mode}, state {state: connected|relay|failed}, leave, and, as a code room host, host {action: admit|deny|remove|mute|unmute|end, peerId}. The newcomer sends the offers. The server pings every 20 seconds; a peer silent for 60 seconds is out of the room.
// @Tags calls
// @Param token query string true "Call token (wsToken)"
// @Success 101 "Switching Protocols"
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /calls/ws [get]
func (ch *CallHandler) Socket(ctx *gin.Context) {
	var callSocketRequest call.CallSocketRequest
	if err := ctx.ShouldBindQuery(&callSocketRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Call token is required"})
		return
	}

	session, err := ch.callController.AuthorizeSocket(ctx.Request.Context(), &callSocketRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrCallTokenInvalid):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Invalid call token"})
		case errors.Is(err, apperrors.ErrCallRoomNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Call has ended"})
		case errors.Is(err, apperrors.ErrCallRoomFull):
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "Call room is full"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to open call socket"})
		}
		return
	}

	conn, err := callSocketUpgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		return
	}

	requestCtx := ctx.Request.Context()
	socket := newCallSocket(conn)
	go socket.write()

	if err := ch.callController.ConnectSocket(requestCtx, session, socket); err != nil {
		reason := "Failed to join the call"
		switch {
		case errors.Is(err, apperrors.ErrCallRoomFull):
			reason = "Call room is full"
		case errors.Is(err, apperrors.ErrCallRoomNotFound):
			reason = "Call has ended"
		case errors.Is(err, apperrors.ErrCallTokenInvalid):
			reason = "Invalid call token"
		default:
			ch.logger.ErrorContext(requestCtx, "call socket connect failed", "error", err)
		}
		_ = socket.Send(call.CallSignalMessage{Type: string(models.CallSignalError), Reason: reason})
		socket.Close()
		return
	}

	conn.SetReadLimit(constant.MaxCallSignalBytes)
	_ = conn.SetReadDeadline(time.Now().Add(constant.CallPeerTimeout))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(constant.CallPeerTimeout))
		live, err := ch.callController.HeartbeatSocket(requestCtx, session)
		if err != nil {
			ch.logger.ErrorContext(requestCtx, "call socket heartbeat failed", "error", err)
			return nil
		}
		if !live {
			_ = socket.Send(call.CallSignalMessage{Type: string(models.CallSignalEnded)})
			socket.Close()
		}
		return nil
	})

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var message call.CallSignalMessage
		if err := json.Unmarshal(data, &message); err != nil {
			_ = socket.Send(call.CallSignalMessage{Type: string(models.CallSignalError), Reason: "message must be JSON"})
			continue
		}
		if err := ch.callController.HandleSignal(requestCtx, session, &message); err != nil {
			ch.logger.ErrorContext(requestCtx, "call signal failed", "type", message.Type, "error", err)
			break
		}
	}

	socket.Close()
	if err := ch.callController.DisconnectSocket(context.WithoutCancel(requestCtx), session); err != nil {
		ch.logger.ErrorContext(requestCtx, "call socket disconnect failed", "error", err)
	}
}

// callSocket writes to one WebSocket from a single goroutine, as gorilla
// requires. Send queues a message; a socket that falls
// constant.CallSignalBuffer messages behind is closed rather than blocking the
// sender.
type callSocket struct {
	conn      *websocket.Conn
	send      chan call.CallSignalMessage
	done      chan struct{}
	closeOnce sync.Once
}

func newCallSocket(conn *websocket.Conn) *callSocket {
	return &callSocket{
		conn: conn,
		send: make(chan call.CallSignalMessage, constant.CallSignalBuffer),
		done: make(chan struct{}),
	}
}

func (cs *callSocket) Send(message call.CallSignalMessage) error {
	select {
	case <-cs.done:
		return errCallSocketClosed
	default:
	}

	select {
	case cs.send <- message:
		return nil
	default:
		cs.Close()
		return errCallSocketBehind
	}
}

func (cs *callSocket) Close() {
	cs.closeOnce.Do(func() { close(cs.done) })
}

func (cs *callSocket) write() {
	ticker := time.NewTicker(constant.CallHeartbeatInterval)
	defer func() {
		ticker.Stop()
		_ = cs.conn.Close()
	}()

	for {
		select {
		case message := <-cs.send:
			if !cs.writeMessage(message) {
				return
			}
		case <-ticker.C:
			if err := cs.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(constant.CallSignalWriteTimeout)); err != nil {
				return
			}
		case <-cs.done:
			for {
				select {
				case message := <-cs.send:
					if !cs.writeMessage(message) {
						return
					}
				default:
					closing := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
					_ = cs.conn.WriteControl(websocket.CloseMessage, closing, time.Now().Add(constant.CallSignalWriteTimeout))
					return
				}
			}
		}
	}
}

func (cs *callSocket) writeMessage(message call.CallSignalMessage) bool {
	_ = cs.conn.SetWriteDeadline(time.Now().Add(constant.CallSignalWriteTimeout))

	return cs.conn.WriteJSON(message) == nil
}
