package protonmail

import (
	"context"
	"net/http"
	"net/url"
)

type Conversation struct {
	ID             string
	Order          int64
	Subject        string
	Senders        []*MessageAddress
	Recipients     []*MessageAddress
	NumMessages    int
	NumUnread      int
	NumAttachments int
	ExpirationTime Timestamp
	TotalSize      int64
	AddressID      string
	LabelIDs       []string
}

func (c *Client) GetConversation(ctx context.Context, id, msgID string) (*Conversation, []*Message, error) {
	v := url.Values{}
	if msgID != "" {
		v.Set("MessageID", msgID)
	}

	req, err := c.newRequest(ctx, http.MethodGet, "/conversations/"+id+"?"+v.Encode(), nil)
	if err != nil {
		return nil, nil, err
	}

	var respData struct {
		resp
		Conversation *Conversation
		Messages     []*Message
	}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return nil, nil, err
	}

	return respData.Conversation, respData.Messages, nil
}
