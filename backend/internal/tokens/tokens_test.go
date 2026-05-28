package tokens

import (
	"crypto/rand"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func setup(t *testing.T) (uuid.UUID, []byte, []byte) {
	t.Helper()
	id := uuid.New()
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	key := []byte("test-signing-key-32-bytes-long!!!")
	return id, nonce, key
}

func TestRoundtrip(t *testing.T) {
	id, nonce, key := setup(t)

	tok, err := Sign(PurposeView, id, nonce, key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	gotID, gotNonce, err := Verify(PurposeView, tok, key)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if gotID != id {
		t.Fatalf("id mismatch: %v != %v", gotID, id)
	}
	if string(gotNonce) != string(nonce) {
		t.Fatalf("nonce mismatch")
	}
}

func TestPurposeMismatch(t *testing.T) {
	id, nonce, key := setup(t)
	tok, _ := Sign(PurposeQR, id, nonce, key)

	_, _, err := Verify(PurposeView, tok, key)
	if !errors.Is(err, ErrBadPurpose) {
		t.Fatalf("expected ErrBadPurpose, got %v", err)
	}
}

func TestTamperedSignature(t *testing.T) {
	id, nonce, key := setup(t)
	tok, _ := Sign(PurposeView, id, nonce, key)

	// Flip one byte in the signature segment.
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("unexpected token shape")
	}
	sigBytes := []byte(parts[2])
	sigBytes[0] = sigBytes[0] ^ 0x01
	parts[2] = string(sigBytes)
	tampered := strings.Join(parts, ".")

	_, _, err := Verify(PurposeView, tampered, key)
	if !errors.Is(err, ErrBadSignature) && !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrBadSignature/ErrInvalid, got %v", err)
	}
}

func TestWrongKey(t *testing.T) {
	id, nonce, key := setup(t)
	tok, _ := Sign(PurposeView, id, nonce, key)

	otherKey := []byte("a-different-32-byte-key-for-test!")
	_, _, err := Verify(PurposeView, tok, otherKey)
	if !errors.Is(err, ErrBadSignature) {
		t.Fatalf("expected ErrBadSignature, got %v", err)
	}
}

func TestMalformed(t *testing.T) {
	_, _, key := setup(t)
	cases := []string{"", "not-a-token", "view.bad", "view.bad.bad", "view.x.x.x"}
	for _, c := range cases {
		c := c
		t.Run(c, func(t *testing.T) {
			if _, _, err := Verify(PurposeView, c, key); err == nil {
				t.Fatal("expected error for malformed token")
			}
		})
	}
}
