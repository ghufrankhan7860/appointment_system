package controller

import (
	"appointment_system/internal/apps/appointment/dtos/call"
	"appointment_system/internal/apps/appointment/models"
)

func toCallJoinResponse(join *models.CallJoin) call.CallJoinResponse {
	response := call.CallJoinResponse{
		RoomID:           join.Room.ID,
		PeerID:           join.Participant.PeerID,
		Role:             string(join.Participant.Role),
		Waiting:          join.Participant.AdmissionStatus == models.CallAdmissionWaiting,
		WSToken:          join.Token,
		WSTokenExpiresAt: join.TokenExpiresAt.UTC(),
		ICEServers:       make([]call.ICEServerResponse, 0, len(join.ICEServers)),
	}
	for _, server := range join.ICEServers {
		response.ICEServers = append(response.ICEServers, call.ICEServerResponse{
			URLs:       server.URLs,
			Username:   server.Username,
			Credential: server.Credential,
		})
	}

	return response
}

func toCallParticipantResponses(state *models.CallRoomState) []call.CallParticipantResponse {
	participants := make([]call.CallParticipantResponse, 0, len(state.Participants))
	for _, presence := range state.Participants {
		participants = append(participants, call.CallParticipantResponse{
			PeerID:          presence.Participant.PeerID,
			DisplayName:     presence.Participant.DisplayName,
			Role:            string(presence.Participant.Role),
			AdmissionStatus: string(presence.Participant.AdmissionStatus),
			MediaMode:       string(presence.Participant.MediaMode),
			MutedByHost:     presence.Participant.MutedByHost,
			InRoom:          presence.InRoom,
		})
	}

	return participants
}

func toCallRoomResponse(room *models.CallRoom, code string) call.CallRoomResponse {
	return call.CallRoomResponse{
		ID:               room.ID,
		Code:             code,
		Title:            room.Title,
		MaxParticipants:  room.MaxParticipants,
		RequiresApproval: room.RequiresApproval,
		ScheduledAt:      room.ScheduledAt,
		Status:           string(room.Status),
		OpenedAt:         room.OpenedAt.UTC(),
		StartedAt:        room.StartedAt,
		EndedAt:          room.EndedAt,
		EndReason:        string(room.EndReason),
		ExpiresAt:        room.ExpiresAt,
	}
}

func toCallSignal(message *call.CallSignalMessage) models.CallSignal {
	return models.CallSignal{
		Type:      models.CallSignalType(message.Type),
		To:        message.To,
		SDP:       message.SDP,
		Candidate: message.Candidate,
		Mode:      models.CallMediaMode(message.Mode),
		State:     models.CallConnectionState(message.State),
		Action:    models.CallHostAction(message.Action),
		PeerID:    message.PeerID,
		Text:      message.Text,
		MessageID: message.MessageID,
		Emoji:     message.Emoji,
		Reacted:   message.Reacted,
		ChatOff:   message.ChatOff,
	}
}

func toCallSignalMessage(signal models.CallSignal) call.CallSignalMessage {
	message := call.CallSignalMessage{
		Type:      string(signal.Type),
		To:        signal.To,
		From:      signal.From,
		SDP:       signal.SDP,
		Candidate: signal.Candidate,
		Mode:      string(signal.Mode),
		State:     string(signal.State),
		Action:    string(signal.Action),
		PeerID:    signal.PeerID,
		Reason:    signal.Reason,
		Text:      signal.Text,
		MessageID: signal.MessageID,
		Emoji:     signal.Emoji,
		Reacted:   signal.Reacted,
		ChatOff:   signal.ChatOff,
	}
	if signal.Peer != nil {
		peer := toCallSignalPeerMessage(*signal.Peer)
		message.Peer = &peer
	}
	for _, peer := range signal.Peers {
		message.Peers = append(message.Peers, toCallSignalPeerMessage(peer))
	}

	return message
}

func toCallSignalPeerMessage(peer models.CallSignalPeer) call.CallSignalPeerMessage {
	return call.CallSignalPeerMessage{
		PeerID:      peer.PeerID,
		DisplayName: peer.DisplayName,
		Role:        string(peer.Role),
		MediaMode:   string(peer.MediaMode),
		MutedByHost: peer.MutedByHost,
	}
}
