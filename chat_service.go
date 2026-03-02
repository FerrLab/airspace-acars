package main

import (
	"context"
	"encoding/json"
	"fmt"

	"airspace-acars/observability"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var chatTracer = observability.Tracer("chat")

type ChatService struct {
	auth *AuthService
}

type ChatMessage struct {
	ID         int     `json:"id"`
	SenderID   int     `json:"sender_id"`
	SenderName string  `json:"sender_name"`
	SenderRole *string `json:"sender_role"`
	Type       string  `json:"type"`
	Message    string  `json:"message"`
	ReadAt     *string `json:"read_at"`
	CreatedAt  string  `json:"created_at"`
}

type MessagesResponse struct {
	Data        []ChatMessage `json:"data"`
	CurrentPage int           `json:"current_page"`
	LastPage    int           `json:"last_page"`
}

func NewChatService(auth *AuthService) *ChatService {
	return &ChatService{auth: auth}
}

func (c *ChatService) GetMessages(page int) (*MessagesResponse, error) {
	_, span := chatTracer.Start(context.Background(), "chat.get_messages",
		trace.WithAttributes(attribute.Int("chat.page", page)))
	defer span.End()

	path := fmt.Sprintf("/api/acars/messages?page=%d", page)
	body, _, err := c.auth.doRequest("GET", path, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	var result MessagesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("parse messages: %w", err)
	}
	return &result, nil
}

func (c *ChatService) SendMessage(message string) (*ChatMessage, error) {
	_, span := chatTracer.Start(context.Background(), "chat.send_message")
	defer span.End()

	payload := map[string]string{"message": message}
	body, status, err := c.auth.doRequest("POST", "/api/acars/message", payload)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	if status >= 400 {
		err := fmt.Errorf("send message: server returned %d", status)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	var result ChatMessage
	if err := json.Unmarshal(body, &result); err != nil {
		// Some APIs wrap the response; try to extract from "data" key
		var wrapped map[string]json.RawMessage
		if json.Unmarshal(body, &wrapped) == nil {
			if data, ok := wrapped["data"]; ok {
				json.Unmarshal(data, &result)
			}
		}
	}
	return &result, nil
}

func (c *ChatService) ConfirmMessage(messageID int) error {
	payload := map[string]int{"message_id": messageID}
	_, _, err := c.auth.doRequest("PUT", "/api/acars/message/confirm", payload)
	return err
}
