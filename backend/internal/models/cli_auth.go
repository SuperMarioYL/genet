package models

import "time"

type CLIAuthRequestRecord struct {
	ID               string     `json:"id"`
	CodeChallenge    string     `json:"codeChallenge"`
	LocalCallbackURL string     `json:"localCallbackURL"`
	State            string     `json:"state"`
	Username         string     `json:"username,omitempty"`
	Email            string     `json:"email,omitempty"`
	AuthCodeHash     string     `json:"authCodeHash,omitempty"`
	ExpiresAt        time.Time  `json:"expiresAt"`
	UsedAt           *time.Time `json:"usedAt,omitempty"`
}

type CLIRefreshSessionRecord struct {
	ID         string     `json:"id"`
	TokenHash  string     `json:"tokenHash"`
	Username   string     `json:"username"`
	Email      string     `json:"email"`
	UserAgent  string     `json:"userAgent"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
}

type CLIAuthStartRequest struct {
	CodeChallenge    string `json:"codeChallenge" binding:"required"`
	LocalCallbackURL string `json:"localCallbackURL" binding:"required"`
	State            string `json:"state" binding:"required"`
}

type CLIAuthStartResponse struct {
	RequestID string    `json:"requestID"`
	LoginURL  string    `json:"loginURL"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type CLIAuthExchangeRequest struct {
	RequestID    string `json:"requestID" binding:"required"`
	Code         string `json:"code" binding:"required"`
	CodeVerifier string `json:"codeVerifier" binding:"required"`
}

type CLIAuthTokenResponse struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
}
