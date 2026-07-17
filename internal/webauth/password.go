// Package webauth is gameops gateway's single-admin auth: bcrypt password
// hashing, HMAC-signed session cookies, and a simple login rate limiter.
// There is deliberately no user table — one password hash in config is the
// whole auth model (see design spec §6).
package webauth

import "golang.org/x/crypto/bcrypt"

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword reports whether password matches hash. A malformed hash
// (e.g. a config typo) is treated as "no match", not a panic or a crash —
// bcrypt.CompareHashAndPassword already returns a plain error in that case.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
