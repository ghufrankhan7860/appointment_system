package services

import (
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/utils"
)

// chat relays a message to everyone admitted, the sender included, under an
// id the server gives it, so every copy names the same message. Nothing is
// stored or logged. While the moderator has chat off only the moderator can
// send.
func (css *CallSessionService) chat(
	roomID uint64,
	participants []*models.CallParticipant,
	self *models.CallParticipant,
	signal models.CallSignal,
	now time.Time,
) error {
	text := strings.TrimSpace(signal.Text)
	if text == "" {
		css.reject(roomID, self, "message is empty")
		return nil
	}
	if utf8.RuneCountInString(text) > constant.MaxCallChatMessageLength {
		css.reject(roomID, self, fmt.Sprintf("message must be at most %d characters", constant.MaxCallChatMessageLength))
		return nil
	}
	if css.isChatOff(roomID) && !moderatesChat(self) {
		css.reject(roomID, self, "the host turned chat off")
		return nil
	}

	messageID, err := utils.NewHexID(constant.CallChatMessageIDBytes)
	if err != nil {
		return fmt.Errorf("new chat message id: %w", err)
	}
	css.hub.Broadcast(roomID, inRoomPeerIDs(participants, 0, now), models.CallSignal{
		Type:      models.CallSignalChat,
		From:      self.PeerID,
		MessageID: messageID,
		Text:      text,
	})

	return nil
}

// chatReaction relays one person adding (Reacted) or taking back a reaction.
// The server keeps no messages, so it cannot check the message exists; the
// clients ignore a reaction to one they don't have.
func (css *CallSessionService) chatReaction(
	roomID uint64,
	participants []*models.CallParticipant,
	self *models.CallParticipant,
	signal models.CallSignal,
	now time.Time,
) {
	if !validChatMessageID(signal.MessageID) {
		css.reject(roomID, self, "messageId is not a chat message")
		return
	}
	if !slices.Contains(models.CallChatReactions, signal.Emoji) {
		css.reject(roomID, self, "emoji must be one of "+strings.Join(models.CallChatReactions, " "))
		return
	}
	if css.isChatOff(roomID) && !moderatesChat(self) {
		css.reject(roomID, self, "the host turned chat off")
		return
	}

	css.hub.Broadcast(roomID, inRoomPeerIDs(participants, 0, now), models.CallSignal{
		Type:      models.CallSignalChatReaction,
		From:      self.PeerID,
		MessageID: signal.MessageID,
		Emoji:     signal.Emoji,
		Reacted:   signal.Reacted,
	})
}

// moderateChat applies the moderator's chat-delete or chat-settings for
// everyone admitted. The caller checks moderatesChat.
func (css *CallSessionService) moderateChat(
	roomID uint64,
	participants []*models.CallParticipant,
	moderator *models.CallParticipant,
	signal models.CallSignal,
	now time.Time,
) {
	relayed := models.CallSignal{Type: signal.Type, From: moderator.PeerID}
	if signal.Type == models.CallSignalChatDelete {
		if !validChatMessageID(signal.MessageID) {
			css.reject(roomID, moderator, "messageId is not a chat message")
			return
		}
		relayed.MessageID = signal.MessageID
	} else {
		css.chatMu.Lock()
		if signal.ChatOff {
			css.chatOff[roomID] = true
		} else {
			delete(css.chatOff, roomID)
		}
		css.chatMu.Unlock()
		relayed.ChatOff = signal.ChatOff
	}

	css.hub.Broadcast(roomID, inRoomPeerIDs(participants, 0, now), relayed)
}

func (css *CallSessionService) isChatOff(roomID uint64) bool {
	css.chatMu.Lock()
	defer css.chatMu.Unlock()

	return css.chatOff[roomID]
}

// forgetChat drops an ended room's chat switch.
func (css *CallSessionService) forgetChat(roomID uint64) {
	css.chatMu.Lock()
	defer css.chatMu.Unlock()

	delete(css.chatOff, roomID)
}

// moderatesChat reports whether the participant runs the room's chat: a code
// room's host, or the doctor in an appointment call.
func moderatesChat(participant *models.CallParticipant) bool {
	return participant.Role == models.CallRoleHost || participant.Role == models.CallRoleDoctor
}

func validChatMessageID(messageID string) bool {
	if len(messageID) != hex.EncodedLen(constant.CallChatMessageIDBytes) {
		return false
	}
	_, err := hex.DecodeString(messageID)

	return err == nil
}
