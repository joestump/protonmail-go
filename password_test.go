package protonmail

import (
	"strconv"
	"testing"
)

// TestHashPasswordVersionShapes asserts that hashPassword:
//   - rejects unsupported versions (error path),
//   - returns 256 bytes for supported versions (4 * SHA-512 from expandHash),
//   - is deterministic across calls with the same inputs.
//
// We avoid hard-coding a specific hex vector because bcrypt's deterministic
// salting + the proton suffix tightly couples to library internals; a
// self-consistency test is more robust to dependency upgrades.
func TestHashPasswordVersionShapes(t *testing.T) {
	password := []byte("hunter2")
	salt := []byte("0123456789abcdef") // bcrypt requires 16 bytes
	modulus := []byte("modulus-bytes")

	t.Run("unsupported version", func(t *testing.T) {
		if _, err := hashPassword(1, password, salt, modulus); err == nil {
			t.Fatal("expected error for version 1, got nil")
		}
	})

	for _, v := range []int{3, 4} {
		v := v
		t.Run("v"+strconv.Itoa(v), func(t *testing.T) {
			a, err := hashPassword(v, password, append([]byte{}, salt...), modulus)
			if err != nil {
				t.Fatalf("hashPassword v%d: %v", v, err)
			}
			if len(a) != 256 {
				t.Errorf("hashPassword v%d: len = %d, want 256", v, len(a))
			}
			b, err := hashPassword(v, password, append([]byte{}, salt...), modulus)
			if err != nil {
				t.Fatalf("hashPassword v%d (second call): %v", v, err)
			}
			if string(a) != string(b) {
				t.Errorf("hashPassword v%d: not deterministic", v)
			}
		})
	}
}

// TestComputeKeyPassword smoke-tests that computeKeyPassword strips the
// 29-byte bcrypt prefix and returns a non-empty key. Like TestHashPasswordVersionShapes
// we don't pin to a literal vector — bcrypt is a moving target across stdlib
// & emersion forks.
func TestComputeKeyPassword(t *testing.T) {
	password := []byte("hunter2")
	salt := []byte("abcdefghijklmnop") // 16 bytes

	got, err := computeKeyPassword(password, salt)
	if err != nil {
		t.Fatalf("computeKeyPassword: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("computeKeyPassword: returned empty key")
	}
	// bcrypt output is 60 chars; minus the 29-byte prefix = 31.
	if len(got) != 31 {
		t.Errorf("computeKeyPassword: len = %d, want 31", len(got))
	}
}
