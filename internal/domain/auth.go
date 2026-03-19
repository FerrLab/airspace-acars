package domain

type Tenant struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	LogoURL   *string  `json:"logo_url"`
	BannerURL *string  `json:"banner_url"`
	Domains   []string `json:"domains"`
}

type DeviceCodeResponse struct {
	UserCode           string `json:"user_code"`
	AuthorizationToken string `json:"authorization_token"`
}

type TokenResponse struct {
	AccessToken string `json:"access_token,omitempty"`
	Status      int    `json:"status"`
	Error       string `json:"error,omitempty"`
}
