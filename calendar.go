package protonmail

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

const calendarPath = "/calendar/v1"

// CalendarFlags is a bitmask of Proton Calendar flags. Concrete values are
// not yet enumerated by this client.
type CalendarFlags int

// Calendar is a single Proton Calendar. Display is non-zero if the calendar
// is currently shown in the user's UI.
type Calendar struct {
	ID          string
	Name        string
	Description string
	Color       string
	// Display is non-zero if the calendar is shown in the UI.
	Display int
	Flags   CalendarFlags
}

// CalendarEventPermissions is a bitmask describing what the current user is
// allowed to do with a CalendarEvent. Concrete values are not yet enumerated
// by this client.
type CalendarEventPermissions int

// CalendarEvent is a single event within a Calendar. The vCard-like payloads
// are carried in SharedEvents and PersonalEvent as encrypted/signed
// CalendarEventCard values.
type CalendarEvent struct {
	ID                string
	CalendarID        string
	CalendarKeyPacket string
	CreateTime        Timestamp
	LastEditTime      Timestamp
	Author            string
	Permissions       CalendarEventPermissions
	SharedKeyPacket   string
	SharedEvents      []CalendarEventCard
	CalendarEvents    interface{}
	PersonalEvent     []CalendarEventCard
}

// CalendarEventCardType describes how a CalendarEventCard is protected.
// Concrete values are not yet enumerated by this client.
type CalendarEventCardType int

// CalendarEventCard is one of the encrypted or signed payloads attached to
// a CalendarEvent. MemberID identifies which calendar member the card was
// written for (empty for shared cards).
type CalendarEventCard struct {
	Type      CalendarEventCardType
	Data      string
	Signature string
	MemberID  string
}

// ListCalendars returns a paginated list of the user's Proton Calendars.
func (c *Client) ListCalendars(ctx context.Context, page, pageSize int) ([]*Calendar, error) {
	v := url.Values{}
	v.Set("Page", strconv.Itoa(page))
	if pageSize > 0 {
		v.Set("PageSize", strconv.Itoa(pageSize))
	}

	req, err := c.newRequest(ctx, http.MethodGet, calendarPath+"?"+v.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var respData struct {
		resp
		Calendars []*Calendar
	}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return nil, err
	}

	return respData.Calendars, nil
}

// CalendarEventFilter narrows the result set returned by ListCalendarEvents.
// Start and End are Unix timestamps in seconds; Timezone is an IANA TZ name.
type CalendarEventFilter struct {
	Start, End     int64
	Timezone       string
	Page, PageSize int
}

// ListCalendarEvents returns a page of events from the calendar identified
// by calendarID, narrowed by filter.
func (c *Client) ListCalendarEvents(ctx context.Context, calendarID string, filter *CalendarEventFilter) ([]*CalendarEvent, error) {
	v := url.Values{}
	v.Set("Start", strconv.FormatInt(filter.Start, 10))
	v.Set("End", strconv.FormatInt(filter.End, 10))
	v.Set("Timezone", filter.Timezone)
	v.Set("Page", strconv.Itoa(filter.Page))
	if filter.PageSize > 0 {
		v.Set("PageSize", strconv.Itoa(filter.PageSize))
	}

	req, err := c.newRequest(ctx, http.MethodGet, calendarPath+"/"+calendarID+"/events?"+v.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var respData struct {
		resp
		Events []*CalendarEvent
	}
	if err := c.doJSON(ctx, req, &respData); err != nil {
		return nil, err
	}

	return respData.Events, nil

}
