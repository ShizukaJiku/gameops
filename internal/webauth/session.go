package webauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const cookieName = "gameops_session"

// SessionManager issues and validates a self-signed session cookie: the
// value is "<expiryUnix>.<hexHMAC(expiryUnix)>". There's no server-side
// session store — the signature alone proves it wasn't tampered with, and
// the expiry field caps how long a stolen cookie stays valid.
type SessionManager struct {
	secret []byte
	ttl    time.Duration
}

func NewSessionManager(secret []byte, ttl time.Duration) *SessionManager {
	return &SessionManager{secret: secret, ttl: ttl}
}

func (m *SessionManager) sign(expiry int64) string {
	mac := hmac.New(sha256.New, m.secret)
	fmt.Fprintf(mac, "%d", expiry)
	return hex.EncodeToString(mac.Sum(nil))
}

// IssueCookie sets the session cookie on w. Secure=true means it will only
// be sent back over HTTPS — correct for gameops gateway, which is always
// served over TLS via autocert (Task 12).
func (m *SessionManager) IssueCookie(w http.ResponseWriter) {
	expiry := time.Now().Add(m.ttl).Unix()
	value := fmt.Sprintf("%d.%s", expiry, m.sign(expiry))
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(expiry, 0),
	})
}

// Validate reports whether r carries a session cookie with a valid
// signature that hasn't expired yet.
func (m *SessionManager) Validate(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	parts := strings.SplitN(c.Value, ".", 2)
	if len(parts) != 2 {
		return false
	}
	expiry, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix() > expiry {
		return false
	}
	expected := m.sign(expiry)
	return hmac.Equal([]byte(expected), []byte(parts[1]))
}
