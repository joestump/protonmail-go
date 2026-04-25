package protonmail

import (
	"bytes"
	"crypto/sha512"
	"math/big"
	"testing"
)

// TestItoaAtoiRoundTrip verifies that atoi(itoa(x, l)) == x for representative
// values. itoa pads to l/8 bytes and reverses; atoi reverses back. The
// composition should be the identity for any *big.Int that fits in l bits.
func TestItoaAtoiRoundTrip(t *testing.T) {
	const bits = 2048
	cases := []*big.Int{
		big.NewInt(0),
		big.NewInt(1),
		big.NewInt(0xdeadbeef),
		new(big.Int).Lsh(big.NewInt(1), 1024), // 2^1024
	}
	for _, want := range cases {
		// itoa mutates its input via reverse(), so pass a fresh copy.
		encoded := itoa(new(big.Int).Set(want), bits)
		if len(encoded) != bits/8 {
			t.Fatalf("itoa: got len %d, want %d", len(encoded), bits/8)
		}
		got := atoi(append([]byte(nil), encoded...)) // atoi also mutates
		if got.Cmp(want) != 0 {
			t.Errorf("round-trip: got %v, want %v", got, want)
		}
	}
}

// TestExpandHash checks expandHash produces 4*64 = 256 bytes formed by
// SHA-512(input || i) for i in [0,3]. This is a pure function — easy to
// validate against a hand-computed reference.
func TestExpandHash(t *testing.T) {
	input := []byte("hello")
	got := expandHash(input)
	if len(got) != 4*sha512.Size {
		t.Fatalf("expandHash: got len %d, want %d", len(got), 4*sha512.Size)
	}
	for i := 0; i < 4; i++ {
		want := sha512.Sum512(append(input, byte(i)))
		chunk := got[i*sha512.Size : (i+1)*sha512.Size]
		if !bytes.Equal(chunk, want[:]) {
			t.Errorf("expandHash chunk %d: mismatch", i)
		}
	}
}

// TestExpandHashEmpty exercises the boundary case of an empty input. The
// output must still be 256 bytes (4 * SHA-512) and each chunk must match
// SHA-512(byte(i)) for i in [0,3].
func TestExpandHashEmpty(t *testing.T) {
	got := expandHash(nil)
	if len(got) != 4*sha512.Size {
		t.Fatalf("expandHash(nil): got len %d, want %d", len(got), 4*sha512.Size)
	}
	for i := 0; i < 4; i++ {
		want := sha512.Sum512([]byte{byte(i)})
		chunk := got[i*sha512.Size : (i+1)*sha512.Size]
		if !bytes.Equal(chunk, want[:]) {
			t.Errorf("expandHash(nil) chunk %d: mismatch", i)
		}
	}
}

// TestDecodeModulusInvalid asserts that decodeModulus rejects garbage input
// rather than panicking. This is the cheap error-path coverage for the
// otherwise crypto-heavy SRP entrypoint.
func TestDecodeModulusInvalid(t *testing.T) {
	if _, err := decodeModulus("not a pgp message"); err == nil {
		t.Fatal("decodeModulus: expected error for non-PGP input, got nil")
	}
}
