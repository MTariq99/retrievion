package models

import "github.com/google/uuid"

type ChatRequest struct {
	ConversationID uuid.UUID `json:"conversationId"`
	Message        string    `json:"message"`
}

type ChatResponse struct {
	ConversationID uuid.UUID  `json:"conversation_id"`
	MessageID      uuid.UUID  `json:"message_id"`
	Answer         string     `json:"answer"`
	Sources        []Citation `json:"sources"`
}
