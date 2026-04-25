package protonmail

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
)

// PrivateKeyFlags is a bitmask describing what a PrivateKey may be used for
// (verify-only, encrypt, or both).
type PrivateKeyFlags int

const (
	// PrivateKeyVerify indicates the key is allowed to verify signatures.
	PrivateKeyVerify PrivateKeyFlags = 1
	// PrivateKeyEncrypt indicates the key is allowed to encrypt and
	// decrypt.
	PrivateKeyEncrypt PrivateKeyFlags = 2
)

// PrivateKey is an armored, locked private key associated with a User or
// Address. The key is decrypted (in memory) by Unlock; PrivateKey itself
// always carries the locked form.
type PrivateKey struct {
	ID         string
	Version    int
	Flags      PrivateKeyFlags
	PrivateKey string
	Fingerprint string
	// Primary is non-zero for the primary key of the user / address.
	Primary int
	// Active is non-zero if the key is currently active (vs revoked or
	// pending activation).
	Active    int
	Token     string
	Signature string
	// TODO: Fingerprints, PublicKey, Activation
}

// Entity parses the armored private key and returns the first OpenPGP entity
// in it. The returned entity is still locked; pass it through openpgp's
// Decrypt or use the unlocking helpers in this package.
func (priv *PrivateKey) Entity() (*openpgp.Entity, error) {
	keyRing, err := openpgp.ReadArmoredKeyRing(strings.NewReader(priv.PrivateKey))
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}
	if len(keyRing) == 0 {
		return nil, errors.New("private key is empty")
	}
	return keyRing[0], nil
}

// RecipientType describes whether a public-key lookup target is a Proton
// (internal) recipient or an external one.
type RecipientType int

const (
	// RecipientInternal is a Proton-hosted recipient.
	RecipientInternal RecipientType = 1
	// RecipientExternal is a non-Proton recipient.
	RecipientExternal = 2
)

// PublicKeyResp is the response shape returned by GetPublicKeys: the
// recipient classification, the MIME type the recipient prefers, and the
// list of usable public keys.
type PublicKeyResp struct {
	RecipientType RecipientType
	MIMEType      string
	Keys          []*PublicKey
}

// PublicKey is a single armored public key returned by GetPublicKeys. Send
// is non-zero if the key is currently usable for sending.
type PublicKey struct {
	// Send is non-zero if the key is currently usable for sending.
	Send      int
	PublicKey string
}

// Entity parses the armored public key and returns the first OpenPGP entity
// in it.
func (pub *PublicKey) Entity() (*openpgp.Entity, error) {
	keyRing, err := openpgp.ReadArmoredKeyRing(strings.NewReader(pub.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("failed to read public key: %w", err)
	}
	if len(keyRing) == 0 {
		return nil, errors.New("public key is empty")
	}
	return keyRing[0], nil
}

// GetPublicKeys retrieves the public keys associated with email. The
// returned PublicKeyResp.RecipientType reports whether the recipient is a
// Proton or external user; Keys is the list of armored public keys.
func (c *Client) GetPublicKeys(ctx context.Context, email string) (*PublicKeyResp, error) {
	v := url.Values{}
	v.Set("Email", email)
	// TODO: Fingerprint

	req, err := c.newRequest(ctx, http.MethodGet, "/keys?"+v.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var respData struct {
		resp
		*PublicKeyResp
	}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return nil, err
	}

	return respData.PublicKeyResp, nil
}
