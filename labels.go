package protonmail

import (
	"context"
	"net/http"
)

// Built-in system label IDs. These are the well-known identifiers Proton
// uses for the standard mailbox folders; user-created labels have their own
// ULID-style IDs returned by ListLabels.
const (
	// LabelInbox is the Inbox system label.
	LabelInbox = "0"
	// LabelAllDraft contains every draft regardless of address.
	LabelAllDraft = "1"
	// LabelAllSent contains every sent message regardless of address.
	LabelAllSent = "2"
	// LabelTrash is the Trash system label.
	LabelTrash = "3"
	// LabelSpam is the Spam system label.
	LabelSpam = "4"
	// LabelAllMail is the "All Mail" virtual folder containing every
	// non-trashed message.
	LabelAllMail = "5"
	// LabelArchive is the Archive system label.
	LabelArchive = "6"
	// LabelSent is the per-address Sent label (compare LabelAllSent).
	LabelSent = "7"
	// LabelDraft is the per-address Drafts label (compare LabelAllDraft).
	LabelDraft = "8"
	// LabelStarred is the Starred system label.
	LabelStarred = "10"
)

// LabelType distinguishes message labels from contact labels.
type LabelType int

const (
	// LabelMessage is a label that applies to messages.
	LabelMessage LabelType = 1
	// LabelContact is a label that applies to contacts.
	LabelContact LabelType = 2
)

// Label is a user-defined or system label/folder.
type Label struct {
	ID    string
	Name  string
	Color string
	// Display is non-zero if the label is shown in the UI.
	Display int
	Type    LabelType
	// Exclusive is non-zero for folder-style labels (a message can be in
	// at most one exclusive label).
	Exclusive int
	// Notify is non-zero if the user should be notified about new messages
	// receiving this label.
	Notify int
	Order  int
}

// ListLabels returns every label visible to the current user, including
// system labels.
func (c *Client) ListLabels(ctx context.Context) ([]*Label, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/labels", nil)
	if err != nil {
		return nil, err
	}

	var respData struct {
		resp
		Labels []*Label
	}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return nil, err
	}

	return respData.Labels, nil
}
