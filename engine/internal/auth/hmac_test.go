package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryNonces struct {
	mu   sync.Mutex
	used map[string]bool
}

func (m *memoryNonces) Use(_ context.Context, _, nonce string, _ time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.used == nil {
		m.used = map[string]bool{}
	}
	if m.used[nonce] {
		return false, nil
	}
	m.used[nonce] = true
	return true, nil
}

func signedRequest(secret []byte, now time.Time, nonce string) *http.Request {
	timestamp := strconv.FormatInt(now.Unix(), 10)
	request := httptest.NewRequest(http.MethodPost, "http://engine/api/v1/projects", strings.NewReader(`{"name":"Demo"}`))
	canonical := strings.Join([]string{timestamp, nonce, "user_1", "POST", request.URL.EscapedPath()}, "\n")
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(canonical))
	request.Header.Set("X-Authenticated-User", "user_1")
	request.Header.Set("X-Auth-Timestamp", timestamp)
	request.Header.Set("X-Auth-Nonce", nonce)
	request.Header.Set("X-Auth-Signature", hex.EncodeToString(mac.Sum(nil)))
	return request
}

func TestMiddlewareAcceptsSignedRequestAndBlocksReplay(t *testing.T) {
	secret := []byte(strings.Repeat("s", 32))
	now := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	nonces := &memoryNonces{}
	a, err := New(secret, nonces, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	called := 0
	handler := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		if UserID(r.Context()) != "user_1" {
			t.Fatal("authenticated owner missing from context")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	nonce := strings.Repeat("a", 48)
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, signedRequest(secret, now, nonce))
	if first.Code != http.StatusNoContent {
		t.Fatalf("first request status = %d, want 204", first.Code)
	}
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, signedRequest(secret, now, nonce))
	if second.Code != http.StatusUnauthorized {
		t.Fatalf("replay status = %d, want 401", second.Code)
	}
	if called != 1 {
		t.Fatalf("handler called %d times, want 1", called)
	}
}

func TestMiddlewareRejectsExpiredTimestamp(t *testing.T) {
	secret := []byte(strings.Repeat("s", 32))
	now := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	a, err := New(secret, &memoryNonces{}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	request := signedRequest(secret, now.Add(-10*time.Minute), strings.Repeat("b", 48))
	response := httptest.NewRecorder()
	a.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("expired request reached handler") })).ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestNewRequiresStrongSecretAndNonceStore(t *testing.T) {
	if _, err := New([]byte("short"), &memoryNonces{}, time.Now); err == nil {
		t.Fatal("expected weak secret rejection")
	}
	if _, err := New([]byte(strings.Repeat("s", 32)), nil, time.Now); err == nil {
		t.Fatal("expected nil nonce store rejection")
	}
}
