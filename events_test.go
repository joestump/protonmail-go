package protonmail

import (
	"encoding/json"
	"testing"
)

// TestEventMessageUnmarshalCreate confirms that EventCreate populates Created
// (a *Message) and leaves Updated nil.
func TestEventMessageUnmarshalCreate(t *testing.T) {
	raw := []byte(`{
		"ID": "msg-1",
		"Action": 1,
		"Message": {"ID": "msg-1", "Subject": "Hello"}
	}`)
	var em EventMessage
	if err := json.Unmarshal(raw, &em); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if em.ID != "msg-1" {
		t.Errorf("ID = %q, want msg-1", em.ID)
	}
	if em.Action != EventCreate {
		t.Errorf("Action = %d, want EventCreate (%d)", em.Action, EventCreate)
	}
	if em.Created == nil {
		t.Fatal("Created is nil; expected populated *Message")
	}
	if em.Created.Subject != "Hello" {
		t.Errorf("Created.Subject = %q, want Hello", em.Created.Subject)
	}
	if em.Updated != nil {
		t.Errorf("Updated = %+v, want nil", em.Updated)
	}
}

// TestEventMessageUnmarshalUpdate confirms that EventUpdate populates Updated
// (a *EventMessageUpdate) and leaves Created nil.
func TestEventMessageUnmarshalUpdate(t *testing.T) {
	raw := []byte(`{
		"ID": "msg-2",
		"Action": 2,
		"Message": {"Time": 1700000150, "Unread": 0}
	}`)
	var em EventMessage
	if err := json.Unmarshal(raw, &em); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if em.Action != EventUpdate {
		t.Errorf("Action = %d, want EventUpdate", em.Action)
	}
	if em.Created != nil {
		t.Errorf("Created = %+v, want nil", em.Created)
	}
	if em.Updated == nil {
		t.Fatal("Updated is nil; expected populated *EventMessageUpdate")
	}
	if em.Updated.Time != 1700000150 {
		t.Errorf("Updated.Time = %d, want 1700000150", em.Updated.Time)
	}
}

// TestEventMessageUnmarshalDelete confirms EventDelete leaves both Created
// and Updated nil.
func TestEventMessageUnmarshalDelete(t *testing.T) {
	raw := []byte(`{"ID": "msg-3", "Action": 0}`)
	var em EventMessage
	if err := json.Unmarshal(raw, &em); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if em.Action != EventDelete {
		t.Errorf("Action = %d, want EventDelete", em.Action)
	}
	if em.Created != nil || em.Updated != nil {
		t.Errorf("Created=%v Updated=%v, both should be nil", em.Created, em.Updated)
	}
}
