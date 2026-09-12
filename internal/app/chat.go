package app

import (
	"context"
	"encoding/json"
	"fmt"

	"airspace-acars/internal/domain"
	"airspace-acars/observability"
)

// GetMessages retrieves paginated chat messages.
func (a *App) GetMessages(page int) (*domain.MessagesResponse, error) {
	_, span := observability.Start(context.Background(), "chat.get_messages",
		"chat.page", page)
	defer span.Finish()

	path := fmt.Sprintf("/api/v2/acars/messages?page=%d", page)
	body, _, err := a.Airspace.DoRequest("GET", path, nil)
	if err != nil {
		span.Fail(err)
		return nil, err
	}

	if len(body) > 0 && body[0] == '<' {
		err := fmt.Errorf("server returned HTML instead of JSON (possible auth redirect or server error)")
		span.Fail(err)
		return nil, err
	}

	var result domain.MessagesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		span.Fail(err)
		return nil, fmt.Errorf("parse messages: %w", err)
	}
	return &result, nil
}

// SendMessage sends a chat message.
func (a *App) SendMessage(message string) (*domain.ChatMessage, error) {
	_, span := observability.Start(context.Background(), "chat.send_message")
	defer span.Finish()

	payload := map[string]string{"message": message}
	body, status, err := a.Airspace.DoRequest("POST", "/api/v2/acars/message", payload)
	if err != nil {
		span.Fail(err)
		return nil, err
	}
	if status >= 400 {
		err := fmt.Errorf("send message: server returned %d", status)
		span.Fail(err)
		return nil, err
	}

	var result domain.ChatMessage
	if err := json.Unmarshal(body, &result); err != nil {
		var wrapped map[string]json.RawMessage
		if json.Unmarshal(body, &wrapped) == nil {
			if data, ok := wrapped["data"]; ok {
				json.Unmarshal(data, &result)
			}
		}
	}
	return &result, nil
}

// ConfirmMessage marks a message as read.
func (a *App) ConfirmMessage(messageID int) error {
	payload := map[string]int{"message_id": messageID}
	_, _, err := a.Airspace.DoRequest("PUT", "/api/v2/acars/message/confirm", payload)
	return err
}
