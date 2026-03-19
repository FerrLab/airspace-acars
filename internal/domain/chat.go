package domain

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
