package webauth

import "testing"

func TestHashAndCheckPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	if hash == "correct-horse-battery-staple" {
		t.Fatal("expected hash to differ from the plaintext password")
	}
	if !CheckPassword(hash, "correct-horse-battery-staple") {
		t.Fatal("expected CheckPassword to accept the correct password")
	}
}

func TestCheckPasswordRejectsWrongPassword(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	if CheckPassword(hash, "wrong-password") {
		t.Fatal("expected CheckPassword to reject an incorrect password")
	}
}

func TestCheckPasswordRejectsMalformedHash(t *testing.T) {
	if CheckPassword("not-a-real-bcrypt-hash", "anything") {
		t.Fatal("expected CheckPassword to reject a malformed hash instead of panicking or matching")
	}
}
