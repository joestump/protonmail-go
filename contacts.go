package protonmail

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
)

// Contact is a single entry in the user's address book. ContactEmails and
// Cards are populated only by GetContact (and ListContactsExport for Cards),
// not by the lighter ListContacts.
type Contact struct {
	ID         string
	Name       string
	UID        string
	Size       int
	CreateTime Timestamp
	ModifyTime Timestamp
	LabelIDs   []string

	// Not when using ListContacts
	ContactEmails []*ContactEmail
	Cards         []*ContactCard
}

// ContactEmailDefaults is a bitmask of "default for ..." flags carried on a
// ContactEmail (e.g. default From, default sign).
type ContactEmailDefaults int

// ContactEmail is a single email address attached to a Contact.
type ContactEmail struct {
	ID        string
	Email     string
	Type      []string
	Defaults  ContactEmailDefaults
	Order     int
	ContactID string
	LabelIDs  []string

	// Only when using ListContactsEmails
	Name string
}

// ContactCardType describes how a vCard payload is protected on a Contact:
// cleartext, encrypted, signed, or both encrypted and signed.
type ContactCardType int

const (
	// ContactCardCleartext is an unprotected vCard payload.
	ContactCardCleartext ContactCardType = iota
	// ContactCardEncrypted is a vCard encrypted to the user.
	ContactCardEncrypted
	// ContactCardSigned is a cleartext vCard with a detached signature.
	ContactCardSigned
	// ContactCardEncryptedAndSigned is an encrypted vCard with a detached
	// signature over the original plaintext.
	ContactCardEncryptedAndSigned
)

// Signed reports whether t indicates a signed card (signed or
// encrypted-and-signed).
func (t ContactCardType) Signed() bool {
	switch t {
	case ContactCardSigned, ContactCardEncryptedAndSigned:
		return true
	default:
		return false
	}
}

// Encrypted reports whether t indicates an encrypted card (encrypted or
// encrypted-and-signed).
func (t ContactCardType) Encrypted() bool {
	switch t {
	case ContactCardEncrypted, ContactCardEncryptedAndSigned:
		return true
	default:
		return false
	}
}

// ContactCard is a vCard payload (Data) carried on a Contact, optionally
// encrypted and/or detached-signed (Signature). Its interpretation is
// governed by Type; use Read to transparently decrypt and verify against a
// key ring.
type ContactCard struct {
	Type      ContactCardType
	Data      string
	Signature string
}

// NewEncryptedContactCard returns a ContactCard whose Data is the vCard read
// from r, encrypted to the recipients in to. If signer is non-nil the card
// is also detached-signed and the type is set to ContactCardEncryptedAndSigned.
func NewEncryptedContactCard(r io.Reader, to []*openpgp.Entity, signer *openpgp.Entity) (*ContactCard, error) {
	// TODO: sign and encrypt at the same time

	var msg, armored bytes.Buffer
	if signer != nil {
		// We'll sign the message later, keep a copy of it
		r = io.TeeReader(r, &msg)
	}

	ciphertext, err := armor.Encode(&armored, "PGP MESSAGE", nil)
	if err != nil {
		return nil, err
	}

	cleartext, err := openpgp.Encrypt(ciphertext, to, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(cleartext, r); err != nil {
		return nil, err
	}
	if err := cleartext.Close(); err != nil {
		return nil, err
	}

	if err := ciphertext.Close(); err != nil {
		return nil, err
	}

	card := &ContactCard{
		Type: ContactCardEncrypted,
		Data: armored.String(),
	}

	if signer != nil {
		var sig bytes.Buffer
		if err := openpgp.ArmoredDetachSignText(&sig, signer, &msg, nil); err != nil {
			return nil, err
		}

		card.Type = ContactCardEncryptedAndSigned
		card.Signature = sig.String()
	}

	return card, nil
}

// NewSignedContactCard returns a ContactCard whose Data is the cleartext
// vCard read from r and Signature is a detached signature over it.
func NewSignedContactCard(r io.Reader, signer *openpgp.Entity) (*ContactCard, error) {
	var msg, sig bytes.Buffer
	r = io.TeeReader(r, &msg)
	if err := openpgp.ArmoredDetachSignText(&sig, signer, r, nil); err != nil {
		return nil, err
	}

	return &ContactCard{
		Type:      ContactCardSigned,
		Data:      msg.String(),
		Signature: sig.String(),
	}, nil
}

type detachedSignatureReader struct {
	md        *openpgp.MessageDetails
	body      io.Reader
	signed    bytes.Buffer
	signature io.Reader
	keyring   openpgp.KeyRing
	eof       bool
}

func (r *detachedSignatureReader) Read(p []byte) (n int, err error) {
	// TODO: check signature and decrypt at the same time

	n, err = r.body.Read(p)
	if err == io.EOF && !r.eof {
		// Check signature
		signer, signatureError := openpgp.CheckArmoredDetachedSignature(r.keyring, &r.signed, r.signature, nil)
		r.md.IsSigned = true
		r.md.SignatureError = signatureError
		if signer != nil {
			r.md.SignedByKeyId = signer.PrimaryKey.KeyId
			r.md.SignedBy = entityPrimaryKey(signer)
		}
		r.eof = true
	}
	return
}

// Read decrypts and verifies card against keyring and returns OpenPGP
// message details whose UnverifiedBody yields the card's plaintext vCard
// payload. For unencrypted cards the body is yielded verbatim; signature
// verification still runs when the card is signed.
func (card *ContactCard) Read(keyring openpgp.KeyRing) (*openpgp.MessageDetails, error) {
	if !card.Type.Encrypted() {
		md := &openpgp.MessageDetails{
			IsEncrypted:    false,
			IsSigned:       false,
			UnverifiedBody: strings.NewReader(card.Data),
		}

		if !card.Type.Signed() {
			return md, nil
		}

		signed := strings.NewReader(card.Data)
		signature := strings.NewReader(card.Signature)
		signer, err := openpgp.CheckArmoredDetachedSignature(keyring, signed, signature, nil)
		md.IsSigned = true
		md.SignatureError = err
		if signer != nil {
			md.SignedByKeyId = signer.PrimaryKey.KeyId
			md.SignedBy = entityPrimaryKey(signer)
		}
		return md, nil
	}

	ciphertextBlock, err := armor.Decode(strings.NewReader(card.Data))
	if err != nil {
		return nil, err
	}

	md, err := openpgp.ReadMessage(ciphertextBlock.Body, keyring, nil, nil)
	if err != nil {
		return nil, err
	}

	if card.Type.Signed() {
		r := &detachedSignatureReader{
			md:        md,
			signature: strings.NewReader(card.Signature),
			keyring:   keyring,
		}
		r.body = io.TeeReader(md.UnverifiedBody, &r.signed)

		md.UnverifiedBody = r
	}

	return md, nil
}

// ContactExport is a Contact represented purely by its raw cards, as
// returned by ListContactsExport. It is the canonical bulk-export shape.
type ContactExport struct {
	ID    string
	Cards []*ContactCard
}

// ContactImport is the per-contact payload submitted to CreateContacts.
// Cards are encrypted and/or signed using the helpers in this package.
type ContactImport struct {
	Cards []*ContactCard
}

// ListContacts returns a paginated list of contacts. ContactEmails and
// Cards are NOT populated on the returned contacts — use GetContact for the
// full record, or ListContactsExport for the bulk-export shape.
func (c *Client) ListContacts(ctx context.Context, page, pageSize int) (total int, contacts []*Contact, err error) {
	v := url.Values{}
	v.Set("Page", strconv.Itoa(page))
	if pageSize > 0 {
		v.Set("PageSize", strconv.Itoa(pageSize))
	}

	req, err := c.newRequest(ctx, http.MethodGet, "/contacts?"+v.Encode(), nil)
	if err != nil {
		return 0, nil, err
	}

	var respData struct {
		resp
		Contacts []*Contact
		Total    int
	}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return 0, nil, err
	}

	return respData.Total, respData.Contacts, nil
}

// ListContactsEmails returns a paginated list of all email addresses across
// the user's contacts. Each ContactEmail.ContactID identifies the parent
// Contact.
func (c *Client) ListContactsEmails(ctx context.Context, page, pageSize int) (total int, emails []*ContactEmail, err error) {
	v := url.Values{}
	v.Set("Page", strconv.Itoa(page))
	if pageSize > 0 {
		v.Set("PageSize", strconv.Itoa(pageSize))
	}

	req, err := c.newRequest(ctx, http.MethodGet, "/contacts/emails?"+v.Encode(), nil)
	if err != nil {
		return 0, nil, err
	}

	var respData struct {
		resp
		ContactEmails []*ContactEmail
		Total         int
	}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return 0, nil, err
	}

	return respData.Total, respData.ContactEmails, nil
}

// ListContactsExport returns a paginated list of contacts in their raw
// card-only export shape, suitable for backup or migration.
func (c *Client) ListContactsExport(ctx context.Context, page, pageSize int) (total int, contacts []*ContactExport, err error) {
	v := url.Values{}
	v.Set("Page", strconv.Itoa(page))
	if pageSize > 0 {
		v.Set("PageSize", strconv.Itoa(pageSize))
	}

	req, err := c.newRequest(ctx, http.MethodGet, "/contacts/export?"+v.Encode(), nil)
	if err != nil {
		return 0, nil, err
	}

	var respData struct {
		resp
		Contacts []*ContactExport
		Total    int
	}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return 0, nil, err
	}

	return respData.Total, respData.Contacts, nil
}

// GetContact fetches a single contact by ID, including its ContactEmails
// and Cards. Returns a wrapped ErrNotFound if no such contact exists.
func (c *Client) GetContact(ctx context.Context, id string) (*Contact, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/contacts/"+id, nil)
	if err != nil {
		return nil, err
	}

	var respData struct {
		resp
		Contact *Contact
	}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return nil, err
	}

	return respData.Contact, nil
}

// CreateContactResp is the per-contact result returned by CreateContacts.
// Index is the position in the request slice; Response carries the created
// Contact or a per-item error.
type CreateContactResp struct {
	Index    int
	Response struct {
		resp
		Contact *Contact
	}
}

// Err returns the per-contact API error from the response, or nil if the
// individual create succeeded.
func (resp *CreateContactResp) Err() error {
	return resp.Response.Err()
}

// CreateContacts creates a batch of contacts. Each request element gets a
// matching CreateContactResp; per-item failures are surfaced via Err on the
// individual response, not as a top-level error.
func (c *Client) CreateContacts(ctx context.Context, contacts []*ContactImport) ([]*CreateContactResp, error) {
	reqData := struct {
		Contacts                  []*ContactImport
		Overwrite, Groups, Labels int
	}{contacts, 0, 0, 0}
	req, err := c.newJSONRequest(ctx, http.MethodPost, "/contacts", &reqData)
	if err != nil {
		return nil, err
	}

	var respData struct {
		resp
		Responses []*CreateContactResp
	}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return nil, err
	}

	return respData.Responses, nil
}

// UpdateContact replaces the cards on the contact identified by id with
// those in contact. Returns the updated Contact.
func (c *Client) UpdateContact(ctx context.Context, id string, contact *ContactImport) (*Contact, error) {
	req, err := c.newJSONRequest(ctx, http.MethodPut, "/contacts/"+id, contact)
	if err != nil {
		return nil, err
	}

	var respData struct {
		resp
		Contact *Contact
	}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return nil, err
	}

	return respData.Contact, nil
}

// DeleteContactResp is the per-contact result returned by DeleteContacts.
// Use Err to test whether the individual delete succeeded.
type DeleteContactResp struct {
	ID       string
	Response struct {
		resp
	}
}

// Err returns the per-contact API error from the response, or nil if the
// individual delete succeeded.
func (resp *DeleteContactResp) Err() error {
	return resp.Response.Err()
}

// DeleteContacts deletes a batch of contacts by ID. Per-item failures are
// surfaced via Err on the individual response, not as a top-level error.
func (c *Client) DeleteContacts(ctx context.Context, ids []string) ([]*DeleteContactResp, error) {
	reqData := struct {
		IDs []string
	}{ids}
	req, err := c.newJSONRequest(ctx, http.MethodPut, "/contacts/delete", &reqData)
	if err != nil {
		return nil, err
	}

	var respData struct {
		resp
		Responses []*DeleteContactResp
	}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return nil, err
	}

	return respData.Responses, nil
}

// DeleteAllContacts deletes every contact in the user's address book. There
// is no undo.
func (c *Client) DeleteAllContacts(ctx context.Context) error {
	req, err := c.newRequest(ctx, http.MethodDelete, "/contacts", nil)
	if err != nil {
		return err
	}

	var respData resp
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return err
	}

	return nil
}
