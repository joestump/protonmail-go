package protonmail

import (
	"context"
	"encoding/json"
	"net/http"
)

// EventRefresh is a bitmask telling the client to discard cached state and
// re-fetch a category from scratch. The server sets this when the delta
// stream cannot describe the change incrementally.
type EventRefresh int

const (
	// EventRefreshMail signals the client should re-fetch all mail state.
	EventRefreshMail EventRefresh = 1 << iota
	// EventRefreshContacts signals the client should re-fetch all contacts.
	EventRefreshContacts
)

// Event is a single page of the change stream returned by GetEvent. Apply
// Messages and Contacts updates in order, then save ID as the cursor for
// the next call. Refresh, when non-zero, indicates a category must be
// re-fetched from scratch instead of patched incrementally.
type Event struct {
	ID       string `json:"EventID"`
	Refresh  EventRefresh
	Messages []*EventMessage
	Contacts []*EventContact
	//ContactEmails
	//Labels
	//User
	//Members
	//Domains
	//Organization
	MessageCounts []*MessageCount
	//ConversationCounts
	//UsedSpace
	Notices []string
}

// EventAction is the kind of change an Event item describes (delete, create,
// update, or — for messages — a flags-only update).
type EventAction int

const (
	// EventDelete indicates the item was deleted.
	EventDelete EventAction = iota
	// EventCreate indicates the item was created.
	EventCreate
	// EventUpdate indicates the full item was updated.
	EventUpdate

	// EventUpdateFlags indicates a message had only its flags / labels
	// updated; Created is nil and Updated carries the diff.
	EventUpdateFlags
)

// EventMessage is a single message-level entry in an Event. Created is set
// for EventCreate; Updated is set for EventUpdate / EventUpdateFlags. For
// EventDelete only ID is meaningful.
type EventMessage struct {
	ID     string
	Action EventAction

	// Only populated for EventCreate
	Created *Message
	// Only populated for EventUpdate or EventUpdateFlags
	Updated *EventMessageUpdate
}

// EventMessageUpdate is the diff payload describing a Message change in an
// EventMessage. Pointer-typed fields distinguish "unchanged" (nil) from
// "explicitly set to zero". For EventUpdateFlags the LabelIDs* fields carry
// the label-set delta; for EventUpdate LabelIDs is the full new list.
type EventMessageUpdate struct {
	Unread       *int
	Type         *MessageType
	Time         Timestamp
	IsReplied    *int
	IsRepliedAll *int
	IsForwarded  *int

	// Only populated for EventUpdateFlags
	LabelIDs        []string
	LabelIDsAdded   []string
	LabelIDsRemoved []string
}

func buildLabelsSet(labelIDs []string) map[string]struct{} {
	set := make(map[string]struct{}, len(labelIDs))
	for _, labelID := range labelIDs {
		set[labelID] = struct{}{}
	}
	return set
}

// DiffLabelIDs computes the set of label IDs added and removed relative to
// current. If the update carries an explicit added/removed pair (the
// flags-only shape), those are returned verbatim; otherwise the full new
// LabelIDs list is diffed against current. Returns (nil, nil) when neither
// shape is present.
func (update *EventMessageUpdate) DiffLabelIDs(current []string) (added, removed []string) {
	if update.LabelIDsAdded != nil && update.LabelIDsRemoved != nil {
		return update.LabelIDsAdded, update.LabelIDsRemoved
	}
	if update.LabelIDs == nil {
		return
	}

	currentSet := buildLabelsSet(current)
	updateSet := buildLabelsSet(update.LabelIDs)
	for labelID := range currentSet {
		if _, ok := updateSet[labelID]; !ok {
			removed = append(removed, labelID)
		}
	}
	for labelID := range updateSet {
		if _, ok := currentSet[labelID]; !ok {
			added = append(added, labelID)
		}
	}
	return
}

// Patch applies update in place to msg. Pointer fields are copied only when
// non-nil; LabelIDs are replaced (full list) or deltaed (added/removed) as
// appropriate.
func (update *EventMessageUpdate) Patch(msg *Message) {
	msg.Time = update.Time
	if update.Unread != nil {
		msg.Unread = *update.Unread
	}
	if update.Type != nil {
		msg.Type = *update.Type
	}
	if update.IsReplied != nil {
		msg.IsReplied = *update.IsReplied
	}
	if update.IsRepliedAll != nil {
		msg.IsRepliedAll = *update.IsRepliedAll
	}
	if update.IsForwarded != nil {
		msg.IsForwarded = *update.IsForwarded
	}

	if update.LabelIDs != nil {
		msg.LabelIDs = update.LabelIDs
	} else if update.LabelIDsAdded != nil && update.LabelIDsRemoved != nil {
		set := buildLabelsSet(msg.LabelIDs)
		for _, labelID := range update.LabelIDsAdded {
			set[labelID] = struct{}{}
		}
		for _, labelID := range update.LabelIDsRemoved {
			delete(set, labelID)
		}
		msg.LabelIDs = make([]string, 0, len(set))
		for labelID := range set {
			msg.LabelIDs = append(msg.LabelIDs, labelID)
		}
	}
}

type rawEventMessage struct {
	ID      string
	Action  EventAction
	Message json.RawMessage `json:",omitempty"`
}

// UnmarshalJSON decodes an EventMessage, dispatching the embedded Message
// payload into Created or Updated based on Action.
func (em *EventMessage) UnmarshalJSON(b []byte) error {
	var raw rawEventMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	em.ID = raw.ID
	em.Action = raw.Action
	switch raw.Action {
	case EventCreate:
		em.Created = new(Message)
		return json.Unmarshal(raw.Message, em.Created)
	case EventUpdate, EventUpdateFlags:
		em.Updated = new(EventMessageUpdate)
		return json.Unmarshal(raw.Message, em.Updated)
	}
	return nil
}

// EventContact is a single contact-level entry in an Event. Contact is set
// for create / update actions; for EventDelete only ID is meaningful.
type EventContact struct {
	ID      string
	Action  EventAction
	Contact *Contact
}

// GetEvent fetches the next page of changes after the given event ID. Pass
// the empty string (or "latest") to obtain the current event ID without any
// changes — the conventional way to seed the cursor at start-up. The
// returned Event.ID is the cursor for the next call.
func (c *Client) GetEvent(ctx context.Context, last string) (*Event, error) {
	if last == "" {
		last = "latest"
	}

	req, err := c.newRequest(ctx, http.MethodGet, "/events/"+last, nil)
	if err != nil {
		return nil, err
	}

	var respData struct {
		resp
		*Event
	}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return nil, err
	}

	return respData.Event, nil
}
