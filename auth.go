package protonmail

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

type authInfoReq struct {
	Username string
}

// AuthInfo carries the SRP session parameters returned by AuthInfo. Its
// fields are intentionally unexported: they are inputs to the SRP handshake
// performed by Auth and are not useful to inspect directly.
type AuthInfo struct {
	version         int
	modulus         string
	serverEphemeral string
	salt            string
	srpSession      string
}

// AuthInfoResp is the wire-level response returned by /auth/info. Most
// callers should use AuthInfo via the Client.AuthInfo helper rather than
// decoding this directly.
type AuthInfoResp struct {
	resp
	AuthInfo
	Version         int
	Modulus         string
	ServerEphemeral string
	Salt            string
	SRPSession      string
}

func (resp *AuthInfoResp) authInfo() *AuthInfo {
	info := &resp.AuthInfo
	info.version = resp.Version
	info.modulus = resp.Modulus
	info.serverEphemeral = resp.ServerEphemeral
	info.salt = resp.Salt
	info.srpSession = resp.SRPSession
	return info
}

// AuthInfo fetches the SRP parameters required to authenticate as username.
// It is the first step of the authentication flow; callers normally pass the
// returned *AuthInfo to Auth. ctx controls cancellation of the underlying
// request.
func (c *Client) AuthInfo(ctx context.Context, username string) (*AuthInfo, error) {
	reqData := &authInfoReq{
		Username: username,
	}

	req, err := c.newJSONRequest(ctx, http.MethodPost, "/auth/info", reqData)
	if err != nil {
		return nil, err
	}

	var respData AuthInfoResp
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return nil, err
	}

	return respData.authInfo(), nil
}

type authReq struct {
	Username        string
	SRPSession      string
	ClientEphemeral string
	ClientProof     string
}

// PasswordMode describes whether the account uses a single login password
// (PasswordSingle) or separate login and mailbox passwords (PasswordTwo).
type PasswordMode int

const (
	// PasswordSingle indicates the same password unlocks login and mailbox.
	PasswordSingle PasswordMode = 1
	// PasswordTwo indicates the account uses a separate mailbox password.
	PasswordTwo = 2
)

// Auth is the result of a successful authentication. It carries the session
// identifiers (UID, AccessToken, RefreshToken) used for subsequent requests,
// the absolute ExpiresAt at which AccessToken stops being valid, and metadata
// about the account's password and 2FA configuration.
type Auth struct {
	// ExpiresAt is the absolute time at which AccessToken stops being valid.
	ExpiresAt    time.Time
	Scope        string
	UID          string
	AccessToken  string
	RefreshToken string
	UserID       string
	EventID      string
	// PasswordMode reports whether the account uses a single or two-password
	// login.
	PasswordMode PasswordMode
	// TwoFactor describes the 2FA methods enabled on the account. Enabled is
	// non-zero if any 2FA method is enabled; TOTP is non-zero if TOTP is
	// enabled.
	TwoFactor struct {
		Enabled int
		U2F     interface{} // TODO
		TOTP    int
	} `json:"2FA"`
}

type authResp struct {
	resp
	Auth
	ExpiresIn   int
	TokenType   string
	ServerProof string
}

func (resp *authResp) auth() *Auth {
	auth := &resp.Auth
	auth.ExpiresAt = time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
	return auth
}

// Auth completes the SRP handshake and exchanges credentials for a session.
// If info is nil it is fetched via AuthInfo. On success the Client retains
// the resulting UID and AccessToken so subsequent calls are authenticated.
//
// On invalid credentials the error wraps ErrUnauthorized; callers should
// check with errors.Is(err, protonmail.ErrUnauthorized).
func (c *Client) Auth(ctx context.Context, username, password string, info *AuthInfo) (*Auth, error) {
	if info == nil {
		var err error
		if info, err = c.AuthInfo(ctx, username); err != nil {
			return nil, err
		}
	}

	proofs, err := srp([]byte(password), info)
	if err != nil {
		return nil, fmt.Errorf("SRP failed during auth: %w", err)
	}

	reqData := &authReq{
		Username:        username,
		SRPSession:      info.srpSession,
		ClientEphemeral: base64.StdEncoding.EncodeToString(proofs.clientEphemeral),
		ClientProof:     base64.StdEncoding.EncodeToString(proofs.clientProof),
	}

	req, err := c.newJSONRequest(ctx, http.MethodPost, "/auth", reqData)
	if err != nil {
		return nil, err
	}

	var respData authResp
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return nil, err
	}

	if err := proofs.VerifyServerProof(respData.ServerProof); err != nil {
		return nil, err
	}

	auth := respData.auth()
	c.uid = auth.UID
	c.accessToken = auth.AccessToken
	return auth, nil
}

// AuthTOTP submits a six-digit TOTP code to satisfy the second factor of
// authentication. It must be called after Auth on accounts that have TOTP
// enabled (see Auth.TwoFactor.TOTP). The returned scope describes the
// permissions of the now-elevated session.
func (c *Client) AuthTOTP(ctx context.Context, code string) (scope string, err error) {
	reqData := struct {
		TwoFactorCode string
	}{
		TwoFactorCode: code,
	}

	req, err := c.newJSONRequest(ctx, http.MethodPost, "/auth/2fa", reqData)
	if err != nil {
		return "", err
	}

	respData := struct {
		resp
		Scope string
	}{}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return "", err
	}

	return respData.Scope, nil
}

type authRefreshReq struct {
	RefreshToken string

	// Unused but required
	ResponseType string
	GrantType    string
	RedirectURI  string
}

// AuthRefresh exchanges expiredAuth.RefreshToken for a fresh session. It is
// the conventional implementation of the WithReAuth callback: when a request
// receives 401 the Client invokes the registered callback, which usually
// calls AuthRefresh and stores the new tokens for the next attempt.
func (c *Client) AuthRefresh(ctx context.Context, expiredAuth *Auth) (*Auth, error) {
	reqData := &authRefreshReq{
		RefreshToken: expiredAuth.RefreshToken,
		ResponseType: "token",
		GrantType:    "refresh_token",
		RedirectURI:  "http://www.protonmail.ch",
	}

	req, err := c.newJSONRequest(ctx, http.MethodPost, "/auth/refresh", reqData)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Pm-Uid", expiredAuth.UID)

	var respData authResp
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return nil, err
	}

	auth := respData.auth()
	//auth.EventID = expiredAuth.EventID
	auth.PasswordMode = expiredAuth.PasswordMode
	return auth, nil
}

// ListKeySalts fetches the per-key salt material the client needs in order
// to derive the password used to unlock each private key. The returned map
// is keyed by key ID; values may be nil for keys that have no salt.
func (c *Client) ListKeySalts(ctx context.Context) (map[string][]byte, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/keys/salts", nil)
	if err != nil {
		return nil, err
	}

	var respData struct {
		resp
		KeySalts []struct {
			ID      string
			KeySalt string
		}
	}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return nil, err
	}

	salts := make(map[string][]byte, len(respData.KeySalts))
	for _, salt := range respData.KeySalts {
		if salt.KeySalt == "" {
			salts[salt.ID] = nil
			continue
		}
		payload, err := base64.StdEncoding.DecodeString(salt.KeySalt)
		if err != nil {
			return nil, fmt.Errorf("failed to decode key salt payload: %w", err)
		}
		salts[salt.ID] = payload
	}

	return salts, nil
}

func unlockEntity(e *openpgp.Entity, passphraseBytes []byte) error {
	var privateKeys []*packet.PrivateKey

	// e.PrivateKey is a signing key
	if e.PrivateKey != nil {
		privateKeys = append(privateKeys, e.PrivateKey)
	}

	// e.Subkeys are encryption keys
	for _, subkey := range e.Subkeys {
		if subkey.PrivateKey != nil {
			privateKeys = append(privateKeys, subkey.PrivateKey)
		}
	}

	for _, priv := range privateKeys {
		if err := priv.Decrypt(passphraseBytes); err != nil {
			return err
		}
	}

	return nil
}

func decryptPrivateKeyToken(key *PrivateKey, userKeyRing openpgp.EntityList) ([]byte, error) {
	block, err := armor.Decode(strings.NewReader(key.Token))
	if err != nil {
		return nil, err
	}

	md, err := openpgp.ReadMessage(block.Body, userKeyRing, nil, nil)
	if err != nil {
		return nil, err
	}

	b, err := io.ReadAll(md.UnverifiedBody)
	if err != nil {
		return nil, err
	}

	// TODO: check signer?
	_, err = openpgp.CheckArmoredDetachedSignature(userKeyRing, bytes.NewReader(b), strings.NewReader(key.Signature), nil)
	return b, err
}

func unlockPrivateKey(key *PrivateKey, userKeyRing openpgp.EntityList, keySalt []byte, passphraseBytes []byte) (*openpgp.Entity, error) {
	entity, err := key.Entity()
	if err != nil {
		return nil, err
	}

	if key.Token != "" {
		passphraseBytes, err = decryptPrivateKeyToken(key, userKeyRing)
	} else if keySalt != nil {
		passphraseBytes, err = computeKeyPassword(passphraseBytes, keySalt)
	}
	if err != nil {
		return nil, err
	}

	if err := unlockEntity(entity, passphraseBytes); err != nil {
		return nil, err
	}

	return entity, nil
}

func (c *Client) unlockKeyRing(keys []*PrivateKey, userKeyRing openpgp.EntityList, keySalts map[string][]byte, passphraseBytes []byte) (openpgp.EntityList, error) {
	var keyRing openpgp.EntityList
	for _, key := range keys {
		if key.Active != 1 {
			continue
		}

		entity, err := unlockPrivateKey(key, userKeyRing, keySalts[key.ID], passphraseBytes)
		if err != nil {
			c.logger.Warn("protonmail: failed to unlock key",
				slog.String("key_id", key.ID),
				slog.String("fingerprint", key.Fingerprint),
				slog.Any("error", err),
			)
			continue
		}

		keyRing = append(keyRing, entity)
	}

	if len(keyRing) == 0 {
		return nil, fmt.Errorf("auth: %w", ErrNoUnlockableKeys)
	}
	return keyRing, nil
}

// Unlock decrypts the user and address private keys using passphrase and the
// per-key salts returned by ListKeySalts, and stores the resulting key ring
// on the Client so encrypted messages can be read and sent. auth is the
// session returned by Auth (or by AuthTOTP-completed flow).
//
// If no key on the account can be unlocked the returned error wraps
// ErrNoUnlockableKeys; callers can check with errors.Is. Address keys that
// fail to unlock are logged at warn level and skipped.
func (c *Client) Unlock(ctx context.Context, auth *Auth, keySalts map[string][]byte, passphrase string) (openpgp.EntityList, error) {
	c.uid = auth.UID
	c.accessToken = auth.AccessToken

	u, err := c.GetCurrentUser(ctx)
	if err != nil {
		return nil, err
	}

	userKeyRing, err := c.unlockKeyRing(u.Keys, nil, keySalts, []byte(passphrase))
	if err != nil {
		return nil, err
	}

	addrs, err := c.ListAddresses(ctx)
	if err != nil {
		return nil, err
	}

	var keyRing openpgp.EntityList
	for _, addr := range addrs {
		addrKeyRing, err := c.unlockKeyRing(addr.Keys, userKeyRing, keySalts, []byte(passphrase))
		if err != nil {
			c.logger.Warn("protonmail: failed to unlock address",
				slog.String("address_id", addr.ID),
				slog.String("email", addr.Email),
				slog.Any("error", err),
			)
			continue
		}

		keyRing = append(keyRing, addrKeyRing...)
	}

	if len(keyRing) == 0 {
		return nil, fmt.Errorf("auth: %w", ErrNoUnlockableKeys)
	}

	c.keyRing = keyRing

	return keyRing, nil
}

// Logout invalidates the current session on the Proton API and, on success,
// clears the Client's cached UID, access token, and key ring.
//
// If the request itself fails (network error, etc.) the local auth state is
// preserved so that callers can retry. Callers should not assume that
// subsequent requests will fail with ErrUnauthorized: endpoints that do not
// require authentication (for example AuthInfo) will still succeed, and
// authenticated requests issued after a successful Logout will simply
// receive a fresh 401 from the server. The Client does not gate calls on
// the cleared local state.
func (c *Client) Logout(ctx context.Context) error {
	req, err := c.newRequest(ctx, http.MethodDelete, "/auth", nil)
	if err != nil {
		return err
	}

	if err := c.doJSON(ctx, req, nil); err != nil {
		return err
	}

	c.uid = ""
	c.accessToken = ""
	c.keyRing = nil
	return nil
}
