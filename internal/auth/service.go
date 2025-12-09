// Package auth
package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Service handles authentication and encryption operations
type Service struct {
	jwtSecret     []byte
	encryptionKey []byte
	tokenExpiry   time.Duration
	adminUsername string
	adminPassword string
}

// Claims represents JWT token claims
type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// LoginRequest represents the login payload
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents the login response
type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// NewService creates a new authentication service
func NewService(jwtSecret, encryptionKey, adminUsername, adminPassword string, tokenExpiry time.Duration) (*Service, error) {
	if len(jwtSecret) < 32 {
		return nil, errors.New("jwt secret must be at least 32 characters")
	}
	if len(encryptionKey) != 32 {
		return nil, errors.New("encryption key must be exactly 32 bytes for AES-256")
	}

	return &Service{
		jwtSecret:     []byte(jwtSecret),
		encryptionKey: []byte(encryptionKey),
		tokenExpiry:   tokenExpiry,
		adminUsername: adminUsername,
		adminPassword: adminPassword,
	}, nil
}

// Login authenticates a user and returns a JWT token
func (s *Service) Login(username, password string) (*LoginResponse, error) {
	// Simple authentication against configured admin credentials
	if username != s.adminUsername || password != s.adminPassword {
		return nil, errors.New("invalid credentials")
	}

	// Generate JWT token
	expiresAt := time.Now().Add(s.tokenExpiry)
	claims := &Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "nms-lite",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to sign token: %w", err)
	}

	return &LoginResponse{
		Token:     tokenString,
		ExpiresAt: expiresAt,
	}, nil
}

// ValidateToken validates a JWT token and returns the claims
func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}

// Encrypt encrypts plaintext data using AES-256-GCM
func (s *Service) Encrypt(plaintext []byte) (string, error) {
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt and prepend nonce
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	// Return base64 encoded string
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts ciphertext using AES-256-GCM
func (s *Service) Decrypt(ciphertextBase64 string) ([]byte, error) {
	// Decode base64
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}

	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	// Extract nonce and ciphertext
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}
