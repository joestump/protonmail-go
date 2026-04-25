package protonmail

import (
	"context"
	"net/http"
	"testing"
)

// TestListMessages exercises Client.ListMessages against the canned
// testdata/messages/list.json fixture.
func TestListMessages(t *testing.T) {
	body := readFixture(t, "messages/list.json")
	var gotMethod, gotPath, gotQuery string
	srv, c, cleanup := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	defer cleanup()
	_ = srv

	ctx := context.Background()
	total, msgs, err := c.ListMessages(ctx, &MessageFilter{Page: 1, PageSize: 50, Label: "0"})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/messages" {
		t.Errorf("path = %q, want /messages", gotPath)
	}
	if gotQuery == "" {
		t.Error("expected non-empty query string with Page/PageSize/Label")
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d, want 2", len(msgs))
	}
	if msgs[0].ID != "msg-1" || msgs[1].ID != "msg-2" {
		t.Errorf("ids = [%q, %q], want [msg-1, msg-2]", msgs[0].ID, msgs[1].ID)
	}
	if msgs[0].Sender == nil || msgs[0].Sender.Address != "alice@example.com" {
		t.Errorf("msg[0].Sender = %+v, want alice@example.com", msgs[0].Sender)
	}
	if msgs[0].Time.Time().Unix() != 1700000000 {
		t.Errorf("msg[0].Time = %d, want 1700000000", int64(msgs[0].Time))
	}
}

// TestGetMessage exercises Client.GetMessage against testdata/messages/get.json.
func TestGetMessage(t *testing.T) {
	body := readFixture(t, "messages/get.json")
	var gotPath string
	_, c, cleanup := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	defer cleanup()

	ctx := context.Background()
	msg, err := c.GetMessage(ctx, "msg-1")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if gotPath != "/messages/msg-1" {
		t.Errorf("path = %q, want /messages/msg-1", gotPath)
	}
	if msg == nil {
		t.Fatal("msg is nil")
	}
	if msg.Body != "Hello world" {
		t.Errorf("body = %q, want Hello world", msg.Body)
	}
}

// TestCountMessages confirms the helper hits /messages/count and decodes the
// counts array shape correctly.
func TestCountMessages(t *testing.T) {
	var gotPath string
	_, c, cleanup := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"Code": 1000,
			"Counts": [
				{"LabelID": "0", "Total": 5, "Unread": 2},
				{"LabelID": "5", "Total": 1, "Unread": 0}
			]
		}`))
	})
	defer cleanup()

	ctx := context.Background()
	counts, err := c.CountMessages(ctx, "")
	if err != nil {
		t.Fatalf("CountMessages: %v", err)
	}
	if gotPath != "/messages/count" {
		t.Errorf("path = %q, want /messages/count", gotPath)
	}
	if len(counts) != 2 {
		t.Fatalf("len(counts) = %d, want 2", len(counts))
	}
	if counts[0].LabelID != "0" || counts[0].Total != 5 || counts[0].Unread != 2 {
		t.Errorf("counts[0] = %+v", counts[0])
	}
}
