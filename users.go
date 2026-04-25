package protonmail

import (
	"context"
	"net/http"
)

// User describes the authenticated Proton account: quota, billing summary,
// and the user-level private keys.
type User struct {
	ID   string
	Name string
	// UsedSpace is the number of bytes currently consumed.
	UsedSpace int64
	// Currency is the ISO currency code for billing (e.g. "EUR").
	Currency string
	// Credit is the prepaid credit on the account, in the smallest unit of
	// Currency.
	Credit int
	// MaxSpace is the storage quota, in bytes.
	MaxSpace int64
	// MaxUpload is the maximum upload size in bytes.
	MaxUpload int
	// Role is the account role; concrete values are not yet enumerated by
	// this client.
	Role int
	// Private is non-zero if the account uses self-managed (private)
	// password mode.
	Private int
	// Subscribed is a bitmask of subscribed Proton services.
	Subscribed int
	// Services is a bitmask of services available to the account.
	Services int
	// Delinquent is non-zero if the account is in a delinquent billing
	// state.
	Delinquent int
	Keys       []*PrivateKey
}

// GetCurrentUser returns the User record for the currently authenticated
// account.
func (c *Client) GetCurrentUser(ctx context.Context) (*User, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/users", nil)
	if err != nil {
		return nil, err
	}

	var respData struct {
		resp
		User *User
	}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return nil, err
	}

	return respData.User, nil
}
