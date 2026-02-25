package main

import (
	"encoding/json"
	"fmt"
)

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
	path := fmt.Sprintf("/api/acars/messages?page=%d", page)
	body, _, err := c.auth.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var result MessagesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse messages: %w", err)
	}
	return &result, nil
}

func (c *ChatService) SendMessage(message string) (*ChatMessage, error) {
	payload := map[string]string{"message": message}
	body, status, err := c.auth.doRequest("POST", "/api/acars/message", payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("send message: server returned %d", status)
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
