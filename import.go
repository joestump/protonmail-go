package protonmail

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
)

// ImportResult is the per-message outcome of a bulk import, keyed by the
// caller-supplied message key. Use Err for a quick "any failure?" check.
type ImportResult map[string]ImportMessageResult

// Err returns the first non-nil per-message error in res, or nil if every
// message imported successfully.
func (res ImportResult) Err() error {
	for _, msgRes := range res {
		if msgRes.Err != nil {
			return msgRes.Err
		}
	}
	return nil
}

// ImportMessageResult is the outcome of a single message in an import batch.
// On success Err is nil and MessageID is the new server-assigned ID.
type ImportMessageResult struct {
	Err       error
	MessageID string
}

// Importer streams a batch of RFC 822 messages to the Proton import endpoint.
// It is constructed by Client.Import, populated by repeated ImportMessage
// calls, and finalised by Commit.
//
// An Importer may not be reused after Commit: a second call to Commit returns
// ErrImporterClosed (matchable via errors.Is). ImportMessage performs its own
// per-key bookkeeping and currently returns a bare string error if called
// twice for the same key or with an unknown key; that error is not
// errors.Is(ErrImporterClosed) and may become a typed error in a future
// version.
type Importer struct {
	pw       *io.PipeWriter
	mw       *multipart.Writer
	uploaded map[string]bool
	closed   bool
	done     <-chan error
	result   <-chan ImportResult
}

// ImportMessage returns an io.Writer to which the caller writes the raw
// RFC 822 bytes of the message identified by key. key must match one of the
// keys in the metadata map passed to Client.Import; calling ImportMessage
// twice with the same key, or with an unknown key, returns an error.
func (imp *Importer) ImportMessage(ctx context.Context, key string) (io.Writer, error) {
	if uploaded, ok := imp.uploaded[key]; !ok {
		return nil, fmt.Errorf("protonmail: unknown import message %q", key)
	} else if uploaded {
		return nil, fmt.Errorf("protonmail: message %q already imported", key)
	}
	imp.uploaded[key] = true

	hdr := make(textproto.MIMEHeader)
	params := map[string]string{
		"name":     key,
		"filename": key + ".eml",
	}
	hdr.Set("Content-Disposition", mime.FormatMediaType("form-data", params))
	hdr.Set("Content-Type", "message/rfc822")
	return imp.mw.CreatePart(hdr)
}

func (imp *Importer) close() error {
	if imp.closed {
		return ErrImporterClosed
	}
	imp.closed = true

	if err := imp.mw.Close(); err != nil {
		return err
	}

	return imp.pw.Close()
}

// Commit finalises the import and waits for the server response. Every key
// in the metadata map must have been written via ImportMessage first;
// otherwise Commit returns an error without contacting the server. Calling
// Commit a second time returns ErrImporterClosed.
func (imp *Importer) Commit(ctx context.Context) (ImportResult, error) {
	if err := imp.close(); err != nil {
		return nil, err
	}

	for key, ok := range imp.uploaded {
		if !ok {
			return nil, fmt.Errorf("protonmail: message %q has not been imported", key)
		}
	}

	if err := <-imp.done; err != nil {
		return nil, err
	}

	return <-imp.result, nil
}

// Import begins a bulk import session. metadata maps caller-chosen keys to
// per-message metadata (Subject, ToList, etc.); the same keys are then used
// to stream RFC 822 bytes via Importer.ImportMessage. Call Commit on the
// returned Importer to finalise.
func (c *Client) Import(ctx context.Context, metadata map[string]*Message) (*Importer, error) {
	pr, pw := io.Pipe()

	mw := multipart.NewWriter(pw)

	done := make(chan error, 1)
	result := make(chan ImportResult, 1)
	go func() {
		defer close(done)
		defer close(result)

		req, err := c.newRequest(ctx, http.MethodPost, "/import", pr)
		if err != nil {
			done <- err
			return
		}
		req.Header.Set("Content-Type", mw.FormDataContentType())

		type messageResp struct {
			Name     string
			Response struct {
				resp
				MessageID string
			}
		}
		var respData struct {
			resp
			Responses []messageResp
		}
		err = c.doJSON(ctx, req, &respData)
		done <- err
		if err != nil {
			return
		}

		res := make(ImportResult, len(respData.Responses))
		for _, msgData := range respData.Responses {
			res[msgData.Name] = ImportMessageResult{
				Err:       msgData.Response.Err(),
				MessageID: msgData.Response.MessageID,
			}
		}
		result <- res
	}()

	// Send metadata
	hdr := make(textproto.MIMEHeader)
	params := map[string]string{"name": "Metadata"}
	hdr.Set("Content-Disposition", mime.FormatMediaType("form-data", params))
	hdr.Set("Content-Type", "application/json")
	metadataWriter, err := mw.CreatePart(hdr)
	if err != nil {
		pw.CloseWithError(fmt.Errorf("protonmail: failed to write metadata: %w", err))
		return nil, err
	}
	if err := json.NewEncoder(metadataWriter).Encode(metadata); err != nil {
		pw.CloseWithError(fmt.Errorf("protonmail: failed to write metadata: %w", err))
		return nil, err
	}

	uploaded := make(map[string]bool, len(metadata))
	for key := range metadata {
		uploaded[key] = false
	}

	return &Importer{
		pw:       pw,
		mw:       mw,
		uploaded: uploaded,
		done:     done,
		result:   result,
	}, nil
}
