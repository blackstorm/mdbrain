package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

const (
	SessionCookieName = "mdbrain-session"
	sessionMaxAge     = 7 * 24 * 60 * 60
)

type Session struct {
	UserID    string `json:"user_id,omitempty"`
	TenantID  string `json:"tenant_id,omitempty"`
	CSRFToken string `json:"csrf_token,omitempty"`
}

type SessionManager struct {
	signingKey []byte
	secure     bool
}

func NewSessionManager(signingKey []byte, secure bool) *SessionManager {
	keyCopy := make([]byte, len(signingKey))
	copy(keyCopy, signingKey)
	return &SessionManager{signingKey: keyCopy, secure: secure}
}

func (m *SessionManager) Load(c *echo.Context) (Session, error) {
	cookie, err := c.Cookie(SessionCookieName)
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return Session{}, nil
		}
		return Session{}, err
	}
	session, err := m.decode(cookie.Value)
	if err != nil {
		return Session{}, nil
	}
	return session, nil
}

func (m *SessionManager) Save(c *echo.Context, session Session) error {
	encoded, err := m.encode(session)
	if err != nil {
		return err
	}
	c.SetCookie(m.newCookie(encoded, sessionMaxAge))
	return nil
}

func (m *SessionManager) Clear(c *echo.Context) {
	c.SetCookie(m.newCookie("", -1))
}

func (m *SessionManager) EnsureCSRF(session *Session) error {
	if strings.TrimSpace(session.CSRFToken) != "" {
		return nil
	}
	token, err := randomHex(32)
	if err != nil {
		return err
	}
	session.CSRFToken = token
	return nil
}

func (m *SessionManager) newCookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   m.secure,
	}
}

func (m *SessionManager) encode(session Session) (string, error) {
	payload, err := json.Marshal(session)
	if err != nil {
		return "", err
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, m.signingKey)
	_, _ = mac.Write(payload)
	sig := mac.Sum(nil)
	sigPart := base64.RawURLEncoding.EncodeToString(sig)
	return payloadPart + "." + sigPart, nil
}

func (m *SessionManager) decode(raw string) (Session, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return Session{}, errors.New("malformed session cookie")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Session{}, err
	}
	providedSig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Session{}, err
	}
	mac := hmac.New(sha256.New, m.signingKey)
	_, _ = mac.Write(payload)
	expectedSig := mac.Sum(nil)
	if !hmac.Equal(providedSig, expectedSig) {
		return Session{}, errors.New("invalid signature")
	}
	var session Session
	if err := json.Unmarshal(payload, &session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func sessionFromContext(c *echo.Context) Session {
	if value := c.Get("session"); value != nil {
		if session, ok := value.(Session); ok {
			return session
		}
	}
	return Session{}
}

func putSession(c *echo.Context, session Session) {
	c.Set("session", session)
	c.Set("session.user_id", session.UserID)
	c.Set("session.tenant_id", session.TenantID)
	c.Set("csrf_token", session.CSRFToken)
}

func stateChangingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
		return true
	default:
		return false
	}
}

func parseCSRFToken(c *echo.Context) string {
	headerToken := strings.TrimSpace(c.Request().Header.Get("X-CSRF-Token"))
	if headerToken != "" {
		return headerToken
	}
	if token := strings.TrimSpace(c.FormValue("__anti-forgery-token")); token != "" {
		return token
	}
	return ""
}
