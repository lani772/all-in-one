package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	maxClockSkew = 5 * time.Minute
	maxNonceAge  = 10 * time.Minute
	maxBodyBytes = 1 << 20
)

var (
	noncePattern = regexp.MustCompile(`^[a-f0-9]{48}$`)
	userPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

type NonceStore interface {
	Use(context.Context, string, string, time.Time) (bool, error)
}

type Authenticator struct {
	secret []byte
	nonces NonceStore
	now    func() time.Time
}

func New(secret []byte, nonces NonceStore, now func() time.Time) (*Authenticator, error) {
	if len(secret) < 32 {
		return nil, errors.New("HMAC secret must contain at least 32 bytes")
	}
	if nonces == nil {
		return nil, errors.New("nonce store is required")
	}
	if now == nil {
		now = time.Now
	}
	return &Authenticator{secret: append([]byte(nil), secret...), nonces: nonces, now: now}, nil
}

func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			writeError(w, http.StatusUnauthorized, "signed requests must not contain a query string")
			return
		}
		userID := r.Header.Get("X-Authenticated-User")
		timestamp := r.Header.Get("X-Auth-Timestamp")
		nonce := r.Header.Get("X-Auth-Nonce")
		signature := r.Header.Get("X-Auth-Signature")
		if !userPattern.MatchString(userID) || !noncePattern.MatchString(nonce) || len(signature) != 64 {
			writeError(w, http.StatusUnauthorized, "missing or invalid authentication headers")
			return
		}
		unix, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid authentication timestamp")
			return
		}
		signedAt := time.Unix(unix, 0)
		now := a.now()
		if signedAt.Before(now.Add(-maxClockSkew)) || signedAt.After(now.Add(maxClockSkew)) {
			writeError(w, http.StatusUnauthorized, "authentication timestamp is outside the allowed clock window")
			return
		}
		bodyReader := r.Body
		if bodyReader == nil {
			bodyReader = http.NoBody
		}
		body, err := io.ReadAll(io.LimitReader(bodyReader, maxBodyBytes+1))
		if err != nil {
			writeError(w, http.StatusBadRequest, "could not read request body")
			return
		}
		if len(body) > maxBodyBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "request body is too large")
			return
		}
		r.Body = io.NopCloser(strings.NewReader(string(body)))
		canonical := strings.Join([]string{timestamp, nonce, userID, strings.ToUpper(r.Method), r.URL.EscapedPath()}, "\n")
		mac := hmac.New(sha256.New, a.secret)
		_, _ = mac.Write([]byte(canonical))
		expected := mac.Sum(nil)
		provided, err := hex.DecodeString(signature)
		if err != nil || !hmac.Equal(expected, provided) {
			writeError(w, http.StatusUnauthorized, "invalid request signature")
			return
		}
		used, err := a.nonces.Use(r.Context(), userID, nonce, now.Add(maxNonceAge))
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "authentication replay protection is unavailable")
			return
		}
		if !used {
			writeError(w, http.StatusUnauthorized, "request nonce has already been used")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDKey{}, userID)))
	})
}

type userIDLey struct{}

func UserID(ctx context.Context) string {
	value, _ := ctx.Value(userIDKey{}).(string)
	return value
}

type SQLNonceStore struct{ DB *sql.DB }

func (s SQLNonceStore) Use(ctx context.Context, ownerID, nonce string, expiresAt time.Time) (bool, error) {
	if s.DB == nil {
		return false, errors.New("nonce database is not configured")
	}
	result, err := s.DB.ExecContext(ctx,
		`INSERT INTO auth_nonces (nonce, owner_id, expires_at) VALUES ($1,$2,$3) ON CONFLICT (nonce) DO NOTHING`,
		nonce, ownerID, expiresAt,
	)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
