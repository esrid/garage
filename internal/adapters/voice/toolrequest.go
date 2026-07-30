package voice

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/esrid/garage/internal/core/tenant"
)

// Why a tool request was refused. Each tool answers in its own response shape —
// the lookup returns found=false objects, the booking tool a different envelope —
// so the preamble reports the reason and lets the caller phrase it.
var (
	ErrToolUnauthorized = errors.New("voice: tool authentication required")
	ErrToolInvalid      = errors.New("voice: invalid tool request")
)

// DecodeToolRequest is the preamble every voice tool shares: no caching, bearer
// authentication resolving the tenant on the server, a bounded body, and a strict
// JSON decode that refuses unknown fields and trailing values.
//
// It exists as one function because it was written three times in this package —
// once inline per tool — and each copy is a place to forget the body cap or the
// DisallowUnknownFields that keeps a model-generated payload from smuggling a
// field past us. A new tool now inherits the whole boundary by calling this.
//
// On success the returned context carries the tenant; WWW-Authenticate is set
// before returning ErrToolUnauthorized so the caller only has to write a body.
func DecodeToolRequest(w http.ResponseWriter, r *http.Request, authenticator *TokenAuthenticator, maxBodyBytes int64, input any) (context.Context, string, error) {
	w.Header().Set("Cache-Control", "no-store")

	ctx, err := authenticator.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		w.Header().Set("WWW-Authenticate", "Bearer")
		return r.Context(), "", ErrToolUnauthorized
	}
	tenantID, err := tenant.IDFromContext(ctx)
	if err != nil {
		w.Header().Set("WWW-Authenticate", "Bearer")
		return r.Context(), "", ErrToolUnauthorized
	}

	if !IsMediaType(r.Header.Get("Content-Type"), "application/json") {
		return ctx, tenantID, ErrToolInvalid
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(input); err != nil {
		return ctx, tenantID, ErrToolInvalid
	}
	if err := EnsureJSONEnd(decoder); err != nil {
		return ctx, tenantID, ErrToolInvalid
	}
	return ctx, tenantID, nil
}

// EnsureJSONEnd rejects a body carrying anything after the value we decoded. Two
// concatenated objects would otherwise pass, with only the first one read.
func EnsureJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}
