package protonmail

import (
	"context"
	"net/http"
	"testing"
)

// TestListContacts exercises Client.ListContacts against
// testdata/contacts/list.json.
func TestListContacts(t *testing.T) {
	body := readFixture(t, "contacts/list.json")
	var gotMethod, gotPath, gotQuery string
	_, c, cleanup := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	defer cleanup()

	ctx := context.Background()
	total, contacts, err := c.ListContacts(ctx, 0, 100)
	if err != nil {
		t.Fatalf("ListContacts: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/contacts" {
		t.Errorf("path = %q, want /contacts", gotPath)
	}
	if gotQuery == "" {
		t.Error("expected non-empty query string")
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(contacts) != 2 {
		t.Fatalf("len(contacts) = %d, want 2", len(contacts))
	}
	if contacts[0].Name != "Alice" {
		t.Errorf("contacts[0].Name = %q, want Alice", contacts[0].Name)
	}
	if contacts[0].CreateTime.Time().Unix() != 1700000000 {
		t.Errorf("contacts[0].CreateTime = %d, want 1700000000", int64(contacts[0].CreateTime))
	}
}
