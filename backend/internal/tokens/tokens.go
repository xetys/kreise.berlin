// Package tokens emits and verifies HMAC-signed tokens for tickets.
//
// Two purposes share the same primitive: "qr" tokens go inside the QR code
// scanned at the door (Phase 8 verifier); "view" tokens go in the magic-link
// URL emailed to the ticket holder. Domain separation via the purpose prefix
// prevents one being replayed as the other.
//
// Format:
//
//	<purpose>.<base64url(ticket_id || nonce)>.<base64url(hmac-sha256)>
//
// where ticket_id is 16 bytes (UUID), nonce is 16 bytes (rotated on
// transfer), and the HMAC is computed over "<purpose>.<payload>" with the
// shared signing key.
package tokens

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	PurposeQR             = "qr"
	PurposeView           = "view"
	PurposeWaitlistClaim  = "wl_claim"
	PurposeWaitlistRemove = "wl_remove"

	nonceLen   = 16
	uuidLen    = 16
	payloadLen = uuidLen + nonceLen
)

var (
	ErrInvalid      = errors.New("tokens: invalid token")
	ErrBadPurpose   = errors.New("tokens: wrong token purpose")
	ErrBadSignature = errors.New("tokens: signature mismatch")
)

// Sign produces a signed token for the given purpose. nonce must be 16 bytes.
func Sign(purpose string, ticketID uuid.UUID, nonce []byte, key []byte) (string, error) {
	if len(nonce) != nonceLen {
		return "", fmt.Errorf("nonce must be %d bytes", nonceLen)
	}
	if len(key) == 0 {
		return "", errors.New("signing key is empty")
	}
	payload := make([]byte, 0, payloadLen)
	payload = append(payload, ticketID[:]...)
	payload = append(payload, nonce...)

	encPayload := base64.RawURLEncoding.EncodeToString(payload)
	signedInput := purpose + "." + encPayload

	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(signedInput))
	sig := mac.Sum(nil)

	return signedInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// Verify checks the token, returns the ticket id and nonce embedded in it.
// Returns ErrBadPurpose if the token's purpose doesn't match.
func Verify(purpose string, token string, key []byte) (uuid.UUID, []byte, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return uuid.Nil, nil, ErrInvalid
	}
	if parts[0] != purpose {
		return uuid.Nil, nil, ErrBadPurpose
	}

	signedInput := parts[0] + "." + parts[1]
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return uuid.Nil, nil, ErrInvalid
	}

	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(signedInput))
	expected := mac.Sum(nil)

	if !hmac.Equal(sig, expected) {
		return uuid.Nil, nil, ErrBadSignature
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(payload) != payloadLen {
		return uuid.Nil, nil, ErrInvalid
	}

	var id uuid.UUID
	copy(id[:], payload[:uuidLen])
	nonce := make([]byte, nonceLen)
	copy(nonce, payload[uuidLen:])
	return id, nonce, nil
}
