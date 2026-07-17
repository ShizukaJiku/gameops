package webauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIssueCookieThenValidateSucceeds(t *testing.T) {
	sm := NewSessionManager([]byte("test-secret"), time.Hour)

	rec := httptest.NewRecorder()
	sm.IssueCookie(rec)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}

	if !sm.Validate(req) {
		t.Fatal("expected Validate to succeed for a freshly issued cookie")
	}
}

func TestValidateFailsWithoutCookie(t *testing.T) {
	sm := NewSessionManager([]byte("test-secret"), time.Hour)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if sm.Validate(req) {
		t.Fatal("expected Validate to fail when no cookie is present")
	}
}

func TestValidateFailsWithTamperedCookie(t *testing.T) {
	sm := NewSessionManager([]byte("test-secret"), time.Hour)

	rec := httptest.NewRecorder()
	sm.IssueCookie(rec)
	cookies := rec.Result().Cookies()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	tampered := *cookies[0]
	tampered.Value = tampered.Value + "x"
	req.AddCookie(&tampered)

	if sm.Validate(req) {
		t.Fatal("expected Validate to fail for a tampered cookie value")
	}
}

func TestValidateFailsWithDifferentSecret(t *testing.T) {
	issuer := NewSessionManager([]byte("secret-a"), time.Hour)
	validator := NewSessionManager([]byte("secret-b"), time.Hour)

	rec := httptest.NewRecorder()
	issuer.IssueCookie(rec)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}

	if validator.Validate(req) {
		t.Fatal("expected Validate to fail when the signing secret differs")
	}
}

func TestValidateFailsAfterExpiry(t *testing.T) {
	sm := NewSessionManager([]byte("test-secret"), -time.Second) // already expired

	rec := httptest.NewRecorder()
	sm.IssueCookie(rec)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}

	if sm.Validate(req) {
		t.Fatal("expected Validate to fail for an expired session")
	}
}
