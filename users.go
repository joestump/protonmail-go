package protonmail

import (
	"context"
	"net/http"
)

type User struct {
	ID         string
	Name       string
	UsedSpace  int64
	Currency   string // e.g. EUR
	Credit     int
	MaxSpace   int64
	MaxUpload  int
	Role       int // TODO
	Private    int
	Subscribed int // TODO
	Services   int // TODO
	Delinquent int
	Keys       []*PrivateKey
}

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
