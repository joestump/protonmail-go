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

type AuthInfo struct {
	version         int
	modulus         string
	serverEphemeral string
	salt            string
	srpSession      string
}

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

type PasswordMode int

const (
	PasswordSingle PasswordMode = 1
	PasswordTwo                 = 2
)

type Auth struct {
	ExpiresAt    time.Time
	Scope        string
	UID          string
	AccessToken  string
	RefreshToken string
	UserID       string
	EventID      string
	PasswordMode PasswordMode
	TwoFactor    struct {
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
