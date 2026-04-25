package protonmail

import (
	"context"
	"net/http"
)

// AddressSend describes whether an Address can be used to send mail and at
// what priority (primary vs secondary).
type AddressSend int

// AddressStatus reports whether an Address is enabled or disabled on the
// account.
type AddressStatus int

// AddressType classifies an Address as the account's original, an alias,
// or a custom-domain address.
type AddressType int

const (
	// AddressSendDisabled indicates the address cannot be used to send.
	AddressSendDisabled AddressSend = iota
	// AddressSendPrimary indicates the primary sending address.
	AddressSendPrimary
	// AddressSendSecondary indicates a non-primary sending address.
	AddressSendSecondary
)

const (
	// AddressDisabled indicates the address is disabled.
	AddressDisabled AddressStatus = iota
	// AddressEnabled indicates the address is enabled.
	AddressEnabled
)

const (
	// AddressOriginal is the account's original signup address.
	AddressOriginal AddressType = iota
	// AddressAlias is a Proton-provided alias of the original.
	AddressAlias
	// AddressCustom is an address on a custom domain.
	AddressCustom
)

// Address is a single email address attached to the Proton account. Keys
// holds the private keys associated with the address (encrypted; see Unlock).
type Address struct {
	ID          string
	DomainID    string
	Email       string
	Send        AddressSend
	// Receive is non-zero if the address is allowed to receive mail.
	Receive     int
	Status      AddressStatus
	Type        AddressType
	Order       int64
	DisplayName string
	// Signature is an HTML signature appended to outgoing mail from this
	// address.
	Signature string
	// HasKeys is non-zero if the address has at least one key.
	HasKeys int
	Keys    []*PrivateKey
}

// ListAddresses returns every address attached to the current account. The
// returned addresses' Keys field carries the (still-encrypted) private keys;
// they are decrypted as a side effect of Unlock.
func (c *Client) ListAddresses(ctx context.Context) ([]*Address, error) {
	// TODO: Page, PageSize
	req, err := c.newRequest(ctx, http.MethodGet, "/addresses", nil)
	if err != nil {
		return nil, err
	}

	var respData struct {
		resp
		Addresses []*Address
	}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return nil, err
	}

	return respData.Addresses, nil
}
