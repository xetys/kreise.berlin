// Package auth handles password hashing, sessions, and the HTTP-level login/
// logout flow plus the request-context middleware.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2idParams holds tunable argon2id parameters. Defaults follow OWASP 2024
// guidance for memory-hard hashing on a server.
type Argon2idParams struct {
	Memory      uint32 // KiB
	Time        uint32 // iterations
	Parallelism uint8
	SaltLen     uint32
	KeyLen      uint32
}

var DefaultArgon2idParams = Argon2idParams{
	Memory:      64 * 1024, // 64 MiB
	Time:        3,
	Parallelism: 4,
	SaltLen:     16,
	KeyLen:      32,
}

// HashPassword produces a self-describing argon2id hash string of the form
// $argon2id$v=19$m=65536,t=3,p=4$<salt>$<key> — version, params, salt, and
// key all encoded so VerifyPassword can re-derive without external state.
func HashPassword(password string) (string, error) {
	return HashPasswordWithParams(password, DefaultArgon2idParams)
}

func HashPasswordWithParams(password string, p Argon2idParams) (string, error) {
	if password == "" {
		return "", errors.New("password is empty")
	}
	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("read random salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Parallelism, p.KeyLen)

	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		p.Memory, p.Time, p.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	)
	return encoded, nil
}

var (
	ErrInvalidHash         = errors.New("invalid argon2id hash")
	ErrIncompatibleVersion = errors.New("incompatible argon2id version")
)

// VerifyPassword returns (true, nil) on a match, (false, nil) on a mismatch,
// and (false, err) only when the encoded hash is malformed. Callers must
// distinguish between the two non-nil outcomes when shaping the response.
func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, ErrInvalidHash
	}
	if version != argon2.Version {
		return false, ErrIncompatibleVersion
	}

	var p Argon2idParams
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &p.Parallelism); err != nil {
		return false, ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, ErrInvalidHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, ErrInvalidHash
	}

	derived := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Parallelism, uint32(len(key)))
	return subtle.ConstantTimeCompare(key, derived) == 1, nil
}
