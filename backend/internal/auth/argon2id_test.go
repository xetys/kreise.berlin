package auth

import (
	"strings"
	"testing"
)

func TestArgon2id_RoundTrip(t *testing.T) {
	hash, err := HashPassword("hunter2")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=") {
		t.Fatalf("hash missing argon2id prefix: %q", hash)
	}

	ok, err := VerifyPassword("hunter2", hash)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Fatal("expected match, got mismatch")
	}
}

func TestArgon2id_Mismatch(t *testing.T) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	ok, err := VerifyPassword("battery staple", hash)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if ok {
		t.Fatal("expected mismatch, got match")
	}
}

func TestArgon2id_RejectsEmpty(t *testing.T) {
	if _, err := HashPassword(""); err == nil {
		t.Fatal("expected error for empty password, got nil")
	}
}

func TestArgon2id_RejectsInvalidEncoding(t *testing.T) {
	cases := []string{
		"",
		"not-a-hash",
		"$argon2id$v=19$$$$",
		"$argon2id$v=19$m=64,t=3,p=4$invalid_b64$abc",
	}
	for _, c := range cases {
		c := c
		t.Run(c, func(t *testing.T) {
			if _, err := VerifyPassword("anything", c); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestArgon2id_DifferentSaltsProduceDifferentHashes(t *testing.T) {
	a, err := HashPassword("same")
	if err != nil {
		t.Fatalf("hash a: %v", err)
	}
	b, err := HashPassword("same")
	if err != nil {
		t.Fatalf("hash b: %v", err)
	}
	if a == b {
		t.Fatal("hashes for the same password must be different (random salt)")
	}
}
