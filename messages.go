package protonmail

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

// MessageType classifies a Message's role in the user's mailbox.
type MessageType int

const (
	// MessageInbox is a received message.
	MessageInbox MessageType = iota
	// MessageDraft is a draft message that has not been sent.
	MessageDraft
	// MessageSent is a message the user has sent.
	MessageSent
	// MessageInboxAndSent is a message that appears in both Inbox and Sent
	// (e.g. messages a user sent to themselves).
	MessageInboxAndSent
)

// MessageEncryption describes how a Message body is protected.
type MessageEncryption int

const (
	// MessageUnencrypted indicates the body is plaintext (no PGP).
	MessageUnencrypted MessageEncryption = iota
	// MessageEncryptedInternal indicates encryption to a Proton recipient.
	MessageEncryptedInternal
	// MessageEncryptedExternal indicates encryption to an external PGP
	// recipient.
	MessageEncryptedExternal
	// MessageEncryptedOutside indicates Proton's "encrypted-for-outside"
	// password-protected delivery.
	MessageEncryptedOutside
	_
	_
	_
	// MessageEncryptedInlinePGP indicates an inline-PGP encrypted body.
	MessageEncryptedInlinePGP
	// MessageEncryptedPGPMIME indicates a PGP/MIME encrypted body.
	MessageEncryptedPGPMIME
	_
	_
)

// MessageAction is the kind of follow-up action that produced a draft (reply,
// reply-all, or forward). Used when creating a draft in response to a parent
// message.
type MessageAction int

const (
	// MessageReply marks the draft as a reply to the parent message.
	MessageReply MessageAction = iota
	// MessageReplyAll marks the draft as a reply-all.
	MessageReplyAll
	// MessageForward marks the draft as a forward of the parent message.
	MessageForward
)

// MessageAddress is a single From/To/CC/BCC participant on a message.
type MessageAddress struct {
	Address string
	Name    string
}

// Message is a Proton mail message — either received, sent, or a draft.
// The Body field's interpretation depends on IsEncrypted; use Read to
// transparently decrypt with a key ring.
type Message struct {
	ID             string `json:",omitempty"`
	Order          int64
	ConversationID string `json:",omitempty"`
	Subject        string
	// Unread is non-zero if the message is unread.
	Unread         int
	Type           MessageType
	Sender         *MessageAddress
	ToList         []*MessageAddress
	Time           Timestamp
	Size           int64
	NumAttachments int
	// IsEncrypted reports how Body is protected. Use Read to decrypt.
	IsEncrypted    MessageEncryption
	ExpirationTime Timestamp
	// IsReplied is non-zero if the user has replied to this message.
	IsReplied int
	// IsRepliedAll is non-zero if the user has reply-all'd this message.
	IsRepliedAll int
	// IsForwarded is non-zero if the user has forwarded this message.
	IsForwarded int
	SpamScore   int
	AddressID   string
	// Body is the (possibly encrypted) message body. Its format is governed
	// by IsEncrypted — use Read to transparently decrypt against a key ring.
	Body        string
	MIMEType    string `json:",omitempty"`
	CCList      []*MessageAddress
	BCCList     []*MessageAddress
	ReplyTos    []*MessageAddress
	Header      string `json:",omitempty"`
	Attachments []*Attachment
	LabelIDs    []string
	ExternalID  string `json:",omitempty"`
}

// Read decrypts msg.Body against keyring and returns the OpenPGP message
// details. For unencrypted messages it returns a synthetic MessageDetails
// whose UnverifiedBody yields msg.Body verbatim. prompt is invoked by the
// underlying openpgp library when a private key is locked; nil is acceptable
// when keyring contains only unlocked keys.
func (msg *Message) Read(keyring openpgp.KeyRing, prompt openpgp.PromptFunction) (*openpgp.MessageDetails, error) {
	switch msg.IsEncrypted {
	case MessageUnencrypted:
		return &openpgp.MessageDetails{
			IsEncrypted:    false,
			IsSigned:       false,
			UnverifiedBody: strings.NewReader(msg.Body),
		}, nil
	default:
		block, err := armor.Decode(strings.NewReader(msg.Body))
		if err != nil {
			return nil, err
		}

		return openpgp.ReadMessage(block.Body, keyring, prompt, nil)
	}
}

type messageWriter struct {
	plaintext  io.WriteCloser
	ciphertext io.WriteCloser
	b          *bytes.Buffer
	msg        *Message
}

func (w *messageWriter) Write(p []byte) (n int, err error) {
	return w.plaintext.Write(p)
}

func (w *messageWriter) Close() error {
	if err := w.plaintext.Close(); err != nil {
		return err
	}
	if err := w.ciphertext.Close(); err != nil {
		return err
	}
	w.msg.Body = w.b.String()
	return nil
}

// Encrypt returns a WriteCloser that encrypts data written to it for the
// given recipients and stores the resulting armored ciphertext on msg.Body
// when the writer is closed. If signed is non-nil the message is also signed
// with that entity. The caller must Close the returned writer.
func (msg *Message) Encrypt(to []*openpgp.Entity, signed *openpgp.Entity) (plaintext io.WriteCloser, err error) {
	var b bytes.Buffer
	ciphertext, err := armor.Encode(&b, "PGP MESSAGE", nil)
	if err != nil {
		return nil, err
	}

	plaintext, err = openpgp.Encrypt(ciphertext, to, signed, nil, nil)
	if err != nil {
		return nil, err
	}

	return &messageWriter{
		plaintext:  plaintext,
		ciphertext: ciphertext,
		b:          &b,
		msg:        msg,
	}, nil
}

// MessageFilter narrows the result set returned by ListMessages. Zero-valued
// fields are omitted from the request. Pointer-typed bool fields
// (Attachments, Starred, Unread) distinguish "unset" (nil) from "explicitly
// false" (pointer to false).
type MessageFilter struct {
	Page     int
	PageSize int
	Limit    int

	Label        string
	Sort         string
	Asc          bool
	Begin        int64
	End          int64
	Keyword      string
	To           string
	From         string
	Subject      string
	Attachments  *bool
	Starred      *bool
	Unread       *bool
	Conversation string
	AddressID    string
	ID           []string
	ExternalID   string
}

// ListMessages returns the page of messages matching filter. total is the
// number of messages matched across all pages, messages is the current page.
// Pass a zero-value *MessageFilter to list with server defaults.
func (c *Client) ListMessages(ctx context.Context, filter *MessageFilter) (total int, messages []*Message, err error) {
	v := url.Values{}
	if filter.Page != 0 {
		v.Set("Page", strconv.Itoa(filter.Page))
	}
	if filter.PageSize != 0 {
		v.Set("PageSize", strconv.Itoa(filter.PageSize))
	}
	if filter.Limit != 0 {
		v.Set("Limit", strconv.Itoa(filter.Limit))
	}
	if filter.Label != "" {
		v.Set("Label", filter.Label)
	}
	if filter.Sort != "" {
		v.Set("Sort", filter.Sort)
	}
	if filter.Asc {
		v.Set("Desc", "0")
	}
	if filter.Conversation != "" {
		v.Set("Conversation", filter.Conversation)
	}
	if filter.AddressID != "" {
		v.Set("AddressID", filter.AddressID)
	}
	if filter.ExternalID != "" {
		v.Set("ExternalID", filter.ExternalID)
	}

	req, err := c.newRequest(ctx, http.MethodGet, "/messages?"+v.Encode(), nil)
	if err != nil {
		return 0, nil, err
	}

	var respData struct {
		resp
		Total    int
		Messages []*Message
	}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return 0, nil, err
	}

	return respData.Total, respData.Messages, nil
}

// MessageCount carries per-label totals for an address: total messages and
// the number unread.
type MessageCount struct {
	LabelID string
	Total   int
	Unread  int
}

// CountMessages returns per-label message counts. If address is empty,
// counts are aggregated across all addresses on the account; otherwise they
// are scoped to that address ID.
func (c *Client) CountMessages(ctx context.Context, address string) ([]*MessageCount, error) {
	v := url.Values{}
	if address != "" {
		v.Set("Address", address)
	}
	req, err := c.newRequest(ctx, http.MethodGet, "/messages/count?"+v.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var respData struct {
		resp
		Counts []*MessageCount
	}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return nil, err
	}

	return respData.Counts, nil
}

// GetMessage fetches a single message by ID, including its body and
// attachment metadata. Returns a wrapped ErrNotFound if no such message
// exists.
func (c *Client) GetMessage(ctx context.Context, id string) (*Message, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/messages/"+id, nil)
	if err != nil {
		return nil, err
	}

	var respData struct {
		resp
		Message *Message
	}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return nil, err
	}

	return respData.Message, nil
}

// CreateDraftMessage creates a draft message.
//
// The Proton API expects msg to carry at minimum a Subject, a sender
// (Sender or AddressID), and a Body; recipients (ToList / CCList / BCCList)
// are required for any draft you intend to send. This client does not
// validate msg before sending — missing fields surface as an *APIError
// from the server.
//
// parentID may be empty for a fresh draft, or set to the ID of a parent
// message to thread the draft as a reply. The current implementation
// hardcodes Action=MessageReply when parentID is non-empty; reply-all and
// forward semantics are not yet wired through (see also SendMessage).
func (c *Client) CreateDraftMessage(ctx context.Context, msg *Message, parentID string) (*Message, error) {
	var actionPtr *MessageAction
	if parentID != "" {
		// TODO: support other actions
		action := MessageReply
		actionPtr = &action
	}

	reqData := struct {
		Message  *Message
		ParentID string         `json:",omitempty"`
		Action   *MessageAction `json:",omitempty"`
	}{msg, parentID, actionPtr}
	req, err := c.newJSONRequest(ctx, http.MethodPost, "/messages", &reqData)
	if err != nil {
		return nil, err
	}

	var respData struct {
		resp
		Message *Message
	}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return nil, err
	}

	return respData.Message, nil
}

// UpdateDraftMessage updates an existing draft. msg.ID must identify the
// draft to update. The returned Message reflects the server's view after
// update.
func (c *Client) UpdateDraftMessage(ctx context.Context, msg *Message) (*Message, error) {
	reqData := struct {
		Message *Message
	}{msg}
	req, err := c.newJSONRequest(ctx, http.MethodPut, "/messages/"+msg.ID, &reqData)
	if err != nil {
		return nil, err
	}

	var respData struct {
		resp
		Message *Message
	}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return nil, err
	}

	return respData.Message, nil
}

func (c *Client) doMessages(ctx context.Context, action string, ids []string) error {
	reqData := struct {
		IDs []string
	}{ids}
	req, err := c.newJSONRequest(ctx, http.MethodPut, "/messages/"+action, &reqData)
	if err != nil {
		return err
	}

	// TODO: the response contains one response per message
	return c.doJSON(ctx, req, nil)
}

// MarkMessagesRead marks the given message IDs as read in a single batch
// request.
func (c *Client) MarkMessagesRead(ctx context.Context, ids []string) error {
	return c.doMessages(ctx, "read", ids)
}

// MarkMessagesUnread marks the given message IDs as unread in a single batch
// request.
func (c *Client) MarkMessagesUnread(ctx context.Context, ids []string) error {
	return c.doMessages(ctx, "unread", ids)
}

// DeleteMessages soft-deletes the given message IDs (move to Trash). Use
// UndeleteMessages to reverse, or call again from the Trash to permanently
// delete.
func (c *Client) DeleteMessages(ctx context.Context, ids []string) error {
	return c.doMessages(ctx, "delete", ids)
}

// UndeleteMessages restores soft-deleted messages by ID.
func (c *Client) UndeleteMessages(ctx context.Context, ids []string) error {
	return c.doMessages(ctx, "undelete", ids)
}

// LabelMessages applies labelID to every message ID in ids. To remove a
// label, use UnlabelMessages.
func (c *Client) LabelMessages(ctx context.Context, labelID string, ids []string) error {
	reqData := struct {
		LabelID string
		IDs     []string
	}{labelID, ids}
	req, err := c.newJSONRequest(ctx, http.MethodPut, "/messages/label", &reqData)
	if err != nil {
		return err
	}

	// TODO: the response contains one response per message
	return c.doJSON(ctx, req, nil)
}

// UnlabelMessages removes labelID from every message ID in ids.
func (c *Client) UnlabelMessages(ctx context.Context, labelID string, ids []string) error {
	reqData := struct {
		LabelID string
		IDs     []string
	}{labelID, ids}
	req, err := c.newJSONRequest(ctx, http.MethodPut, "/messages/unlabel", &reqData)
	if err != nil {
		return err
	}

	// TODO: the response contains one response per message
	return c.doJSON(ctx, req, nil)
}

// MessageKeyPacket pairs a message ID with its base64-encoded key packets.
type MessageKeyPacket struct {
	ID         string
	KeyPackets string
}

// MessagePackageType is a bitmask describing how a single recipient's
// message package is encoded. It is OR'd into MessagePackageSet.Type to
// describe all delivery modes used in a send.
type MessagePackageType int

const (
	// MessagePackageInternal is delivery to a Proton recipient using the
	// Proton internal protocol.
	MessagePackageInternal MessagePackageType = 1
	// MessagePackageEncryptedOutside is Proton's encrypted-for-outside
	// password-protected delivery.
	MessagePackageEncryptedOutside = 2
	// MessagePackageCleartext is unencrypted delivery to an external
	// recipient.
	MessagePackageCleartext = 4
	// MessagePackageInlinePGP is inline-PGP delivery to a non-Proton PGP
	// recipient.
	MessagePackageInlinePGP = 8
	// MessagePackagePGPMIME is PGP/MIME delivery to a non-Proton PGP
	// recipient.
	MessagePackagePGPMIME = 16
	// MessagePackageMIME indicates a MIME-formatted package (not separately
	// encrypted by Proton).
	MessagePackageMIME = 32
)

// From https://github.com/ProtonMail/WebClient/blob/public/src/app/composer/services/encryptMessage.js

// MessagePackage carries the per-recipient encryption material for an
// outgoing message: the body key packet and any attachment key packets,
// encrypted to that recipient's key.
type MessagePackage struct {
	Type MessagePackageType

	BodyKeyPacket        string            `json:",omitempty"`
	AttachmentKeyPackets map[string]string `json:",omitempty"`
	Signature            int
}

// MessagePackageSet is the per-MIME-type container of recipient packages
// that, together, make up an outgoing message. It is constructed by
// NewMessagePackageSet, populated via Encrypt and AddCleartext / AddInternal,
// and carried inside an OutgoingMessage to SendMessage.
type MessagePackageSet struct {
	// Type is the OR of each recipient package's Type, describing all
	// delivery modes used.
	Type      MessagePackageType
	Addresses map[string]*MessagePackage
	MIMEType  string
	// Body is the encrypted body data packet (base64-encoded).
	Body string

	// BodyKey and AttachmentKeys are populated only when at least one
	// cleartext recipient has been added; they carry the symmetric keys in
	// the clear so the API can forward them to the recipient.
	BodyKey        *PackedKey            `json:",omitempty"`
	AttachmentKeys map[string]*PackedKey `json:",omitempty"`

	bodyKey        *packet.EncryptedKey
	attachmentKeys map[string]*packet.EncryptedKey
	signature      int
}

// PackedKey is a base64-encoded symmetric key alongside the cipher algorithm
// it was generated for (e.g. "aes256").
type PackedKey struct {
	Algorithm string
	Key       string
}

// NewMessagePackageSet constructs an empty MessagePackageSet sharing the
// supplied attachment keys. Call Encrypt to populate the body, then add
// recipient packages via AddInternal (Proton recipients) or AddCleartext
// (external unencrypted recipients).
func NewMessagePackageSet(attachmentKeys map[string]*packet.EncryptedKey) *MessagePackageSet {
	return &MessagePackageSet{
		Addresses:      make(map[string]*MessagePackage),
		attachmentKeys: attachmentKeys,
	}
}

type outgoingMessageWriter struct {
	cleartext  io.WriteCloser
	ciphertext io.WriteCloser
	encoded    *bytes.Buffer
	set        *MessagePackageSet
}

func (w *outgoingMessageWriter) Write(p []byte) (int, error) {
	return w.cleartext.Write(p)
}

func (w *outgoingMessageWriter) Close() error {
	if err := w.cleartext.Close(); err != nil {
		return err
	}
	if err := w.ciphertext.Close(); err != nil {
		return err
	}
	w.set.Body = w.encoded.String()
	w.encoded = nil
	return nil
}

// Encrypt returns a WriteCloser that encrypts the data written to it under
// a freshly-generated symmetric key, optionally signs it with signed, and
// stores the resulting base64-encoded ciphertext on set.Body when closed.
// mimeType is recorded on the set for downstream packaging.
func (set *MessagePackageSet) Encrypt(mimeType string, signed *openpgp.Entity) (io.WriteCloser, error) {
	set.MIMEType = mimeType

	config := &packet.Config{}

	key, err := generateUnencryptedKey(packet.CipherAES256, config)
	if err != nil {
		return nil, err
	}
	set.bodyKey = key

	var signer *packet.PrivateKey
	if signed != nil {
		signKey, ok := signingKey(signed, config.Now())
		if !ok {
			return nil, errors.New("no valid signing keys")
		}
		signer = signKey.PrivateKey
		if signer == nil {
			return nil, errors.New("no private key in signing key")
		}
		if signer.Encrypted {
			return nil, errors.New("signing key must be decrypted")
		}
		set.signature = 1
	}

	encoded := new(bytes.Buffer)
	ciphertext := base64.NewEncoder(base64.StdEncoding, encoded)

	cleartext, err := symetricallyEncrypt(ciphertext, key, signer, nil, config)
	if err != nil {
		return nil, err
	}

	return &outgoingMessageWriter{
		cleartext:  cleartext,
		ciphertext: ciphertext,
		encoded:    encoded,
		set:        set,
	}, nil
}

func cipherFunctionString(cipherFunc packet.CipherFunction) string {
	switch cipherFunc {
	case packet.CipherAES128:
		return "aes128"
	case packet.CipherAES192:
		return "aes192"
	case packet.CipherAES256:
		return "aes256"
	default:
		panic("protonmail: unsupported cipher function")
	}
}

// AddCleartext registers an external recipient that should receive the
// message in cleartext. It also populates BodyKey and AttachmentKeys on the
// set the first time it is called, since cleartext recipients need the
// symmetric keys delivered separately.
func (set *MessagePackageSet) AddCleartext(addr string) (*MessagePackage, error) {
	pkg := &MessagePackage{
		Type:      MessagePackageCleartext,
		Signature: set.signature,
	}
	set.Addresses[addr] = pkg
	set.Type |= MessagePackageCleartext

	if set.BodyKey == nil || set.AttachmentKeys == nil {
		set.BodyKey = &PackedKey{
			Algorithm: cipherFunctionString(set.bodyKey.CipherFunc),
			Key:       base64.StdEncoding.EncodeToString(set.bodyKey.Key),
		}

		set.AttachmentKeys = make(map[string]*PackedKey, len(set.attachmentKeys))
		for att, key := range set.attachmentKeys {
			set.AttachmentKeys[att] = &PackedKey{
				Algorithm: cipherFunctionString(key.CipherFunc),
				Key:       base64.StdEncoding.EncodeToString(key.Key),
			}
		}
	}

	return pkg, nil
}

func serializeEncryptedKey(symKey *packet.EncryptedKey, pub *packet.PublicKey, config *packet.Config) (string, error) {
	var encoded bytes.Buffer
	ciphertext := base64.NewEncoder(base64.StdEncoding, &encoded)

	err := packet.SerializeEncryptedKey(ciphertext, pub, symKey.CipherFunc, symKey.Key, config)
	if err != nil {
		return "", err
	}

	ciphertext.Close()

	return encoded.String(), nil
}

// AddInternal registers a Proton recipient at addr whose body and attachment
// key packets are encrypted to pub. Encrypt must have been called on the set
// first to generate the symmetric keys.
func (set *MessagePackageSet) AddInternal(addr string, pub *openpgp.Entity) (*MessagePackage, error) {
	config := &packet.Config{}

	encKey, ok := encryptionKey(pub, config.Now())
	if !ok {
		return nil, errors.New("cannot encrypt a message to key id " + strconv.FormatUint(pub.PrimaryKey.KeyId, 16) + " because it has no encryption keys")
	}

	bodyKey, err := serializeEncryptedKey(set.bodyKey, encKey.PublicKey, config)
	if err != nil {
		return nil, err
	}

	attachmentKeys := make(map[string]string, len(set.attachmentKeys))
	for att, key := range set.attachmentKeys {
		attKey, err := serializeEncryptedKey(key, encKey.PublicKey, config)
		if err != nil {
			return nil, err
		}
		attachmentKeys[att] = attKey
	}

	set.Type |= MessagePackageInternal
	pkg := &MessagePackage{
		Type:                 MessagePackageInternal,
		BodyKeyPacket:        bodyKey,
		AttachmentKeyPackets: attachmentKeys,
		Signature:            set.signature,
	}
	set.Addresses[addr] = pkg
	return pkg, nil
}

// OutgoingMessage is the payload submitted to SendMessage. ID identifies the
// draft that is being sent; Packages is the per-MIME-type set of recipient
// packages constructed via NewMessagePackageSet.
type OutgoingMessage struct {
	// ID is the draft message ID being sent. The draft must already exist
	// (see CreateDraftMessage / UpdateDraftMessage).
	ID string

	// ExpirationTime, if non-zero, sets a self-destruct duration in seconds
	// after which Proton will delete the message.
	ExpirationTime int

	Packages []*MessagePackageSet
}

// SendMessage sends a draft. msg.ID must identify an existing draft
// (CreateDraftMessage). On success it returns the now-sent Message and, if
// the draft is threaded to a parent, the parent message reflecting any
// IsReplied / IsForwarded updates returned by the server.
//
// SendMessage itself does not distinguish reply / reply-all / forward — the
// MessageAction is fixed when the draft is created via CreateDraftMessage
// (currently always MessageReply when a parentID is supplied). SendMessage
// just POSTs the OutgoingMessage payload to the existing draft ID.
func (c *Client) SendMessage(ctx context.Context, msg *OutgoingMessage) (sent, parent *Message, err error) {
	req, err := c.newJSONRequest(ctx, http.MethodPost, "/messages/"+msg.ID, msg)
	if err != nil {
		return nil, nil, err
	}

	var respData struct {
		resp
		Sent, Parent *Message
	}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return nil, nil, err
	}

	return respData.Sent, respData.Parent, nil
}
