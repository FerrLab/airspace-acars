package domain

// CompanyNOTAM is the read-only notice shown to a pilot.
type CompanyNOTAM struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

// Status is a UI translation key suffix, never an upstream error body.
type NOTAMPage struct {
	Status      string         `json:"status"`
	Data        []CompanyNOTAM `json:"data"`
	CurrentPage int            `json:"current_page"`
	LastPage    int            `json:"last_page"`
}

type NOTAMDetail struct {
	Status string        `json:"status"`
	Data   *CompanyNOTAM `json:"data"`
}
