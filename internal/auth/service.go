package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/thinhnguyen/snapgram/internal/store"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

type Service struct {
	repo   store.Repository
	tokens *TokenService
}

func NewService(repo store.Repository, tokens *TokenService) *Service {
	return &Service{repo: repo, tokens: tokens}
}

func (s *Service) Register(ctx context.Context, username, email, password string) (store.User, string, error) {
	hash, err := hashPassword(password)
	if err != nil {
		return store.User{}, "", err
	}
	user, err := s.repo.CreateUser(ctx, store.CreateUserParams{
		Username:     username,
		Email:        email,
		PasswordHash: hash,
	})
	if err != nil {
		return store.User{}, "", err
	}
	token, err := s.tokens.Issue(user.ID)
	return user, token, err
}

func (s *Service) Login(ctx context.Context, email, password string) (store.User, string, error) {
	user, err := s.repo.FindUserByEmail(ctx, email)
	if err != nil {
		return store.User{}, "", ErrInvalidCredentials
	}
	if !verifyPassword(user.PasswordHash, password) {
		return store.User{}, "", ErrInvalidCredentials
	}
	token, err := s.tokens.Issue(user.ID)
	return user, token, err
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	sum := sha256.Sum256(append(salt, []byte(password)...))
	return hex.EncodeToString(salt) + ":" + hex.EncodeToString(sum[:]), nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, ":")
	if len(parts) != 2 {
		return false
	}
	salt, err := hex.DecodeString(parts[0])
	if err != nil {
		return false
	}
	expected, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}
	actual := sha256.Sum256(append(salt, []byte(password)...))
	return hmac.Equal(expected, actual[:])
}

type TokenService struct {
	secret []byte
	ttl    time.Duration
}

func NewTokenService(secret string, ttl time.Duration) *TokenService {
	return &TokenService{secret: []byte(secret), ttl: ttl}
}

func (s *TokenService) Issue(userID int64) (string, error) {
	expires := time.Now().Add(s.ttl).Unix()
	payload := itoa(userID) + ":" + itoa(expires)
	signature := sign(payload, s.secret)
	return base64.RawURLEncoding.EncodeToString([]byte(payload + ":" + signature)), nil
}

func (s *TokenService) Parse(token string) (int64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, store.ErrUnauthorized
	}
	parts := strings.Split(string(raw), ":")
	if len(parts) != 3 {
		return 0, store.ErrUnauthorized
	}
	payload := parts[0] + ":" + parts[1]
	if !hmac.Equal([]byte(parts[2]), []byte(sign(payload, s.secret))) {
		return 0, store.ErrUnauthorized
	}
	expires, ok := parseInt(parts[1])
	if !ok || time.Now().Unix() > expires {
		return 0, store.ErrUnauthorized
	}
	userID, ok := parseInt(parts[0])
	if !ok {
		return 0, store.ErrUnauthorized
	}
	return userID, nil
}

func sign(payload string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func parseInt(value string) (int64, bool) {
	var out int64
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0, false
		}
		out = out*10 + int64(ch-'0')
	}
	return out, true
}

func itoa(value int64) string {
	if value == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
	}
	return string(buf[i:])
}
