package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/uc-package/genet/internal/models"
)

const (
	cliAccessTokenType       = "cli_access"
	defaultCLIAccessTokenTTL = 15 * time.Minute
)

type CLIAccessClaims struct {
	Username  string `json:"username"`
	Email     string `json:"email"`
	SessionID string `json:"sessionId"`
	TokenType string `json:"tokenType"`
	jwt.RegisteredClaims
}

func createCLIAccessToken(cfg *models.Config, username, email, sessionID string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = defaultCLIAccessTokenTTL
	}
	now := time.Now().UTC()
	claims := CLIAccessClaims{
		Username:  username,
		Email:     email,
		SessionID: sessionID,
		TokenType: cliAccessTokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    "genet-cli",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(cfg.OAuth.JWTSecret))
}

func validateCLIAccessToken(cfg *models.Config, tokenString string) (*CLIAccessClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &CLIAccessClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(cfg.OAuth.JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*CLIAccessClaims)
	if !ok || !token.Valid || claims.TokenType != cliAccessTokenType || strings.TrimSpace(claims.Username) == "" {
		return nil, jwt.ErrSignatureInvalid
	}
	return claims, nil
}

func generateCLISecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func buildPKCEChallenge(codeVerifier string) string {
	sum := sha256.Sum256([]byte(codeVerifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func validatePKCEChallenge(codeVerifier, codeChallenge string) error {
	if buildPKCEChallenge(codeVerifier) != codeChallenge {
		return fmt.Errorf("invalid code verifier")
	}
	return nil
}

func GenerateCLISecretForHandler() (string, error) {
	return generateCLISecret()
}

func CreateCLIAccessTokenForHandler(cfg *models.Config, username, email, sessionID string) (string, error) {
	return createCLIAccessToken(cfg, username, email, sessionID, defaultCLIAccessTokenTTL)
}

func DefaultCLIAccessTokenTTLForHandler() time.Duration {
	return defaultCLIAccessTokenTTL
}

func ValidatePKCEChallengeForHandler(codeVerifier, codeChallenge string) error {
	return validatePKCEChallenge(codeVerifier, codeChallenge)
}
