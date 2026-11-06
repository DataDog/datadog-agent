// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(APL) Fix revive linter
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"slices"
	"time"
)

// RequestMetadata contains the request values exposed by net/http. Header names
// and transfer framing have already been parsed; this is not a wire capture.
// Route is the payload-store key, while Target retains the request URI.
type RequestMetadata struct {
	Method           string      `json:"method,omitempty"`
	Target           string      `json:"target,omitempty"`
	Host             string      `json:"host,omitempty"`
	Route            string      `json:"route,omitempty"`
	Protocol         string      `json:"protocol,omitempty"`
	Headers          http.Header `json:"headers,omitempty"`
	Trailers         http.Header `json:"trailers,omitempty"`
	ContentLength    int64       `json:"content_length,omitempty"`
	TransferEncoding []string    `json:"transfer_encoding,omitempty"`
}

// Clone returns independent header, trailer and transfer-encoding values.
func (r *RequestMetadata) Clone() *RequestMetadata {
	if r == nil {
		return nil
	}
	cloned := *r
	cloned.Headers = r.Headers.Clone()
	cloned.Trailers = r.Trailers.Clone()
	cloned.TransferEncoding = slices.Clone(r.TransferEncoding)
	return &cloned
}

//nolint:revive // TODO(APL) Fix revive linter
type Payload struct {
	Timestamp   time.Time        `json:"timestamp"`
	APIKey      string           `json:"api_key"`
	Data        []byte           `json:"data"`
	Encoding    string           `json:"encoding"`
	ContentType string           `json:"content_type"`
	Request     *RequestMetadata `json:"request,omitempty"`
}

// Clone returns an independent body and request metadata snapshot.
func (p Payload) Clone() Payload {
	p.Data = bytes.Clone(p.Data)
	p.Request = p.Request.Clone()
	return p
}

//nolint:revive // TODO(APL) Fix revive linter
type ParsedPayload struct {
	Timestamp time.Time        `json:"timestamp"`
	APIKey    string           `json:"api_key"`
	Data      interface{}      `json:"data"`
	Encoding  string           `json:"encoding"`
	Request   *RequestMetadata `json:"request,omitempty"`
}

//nolint:revive // TODO(APL) Fix revive linter
type APIFakeIntakePayloadsRawGETResponse struct {
	Payloads []Payload `json:"payloads"`
}

//nolint:revive // TODO(APL) Fix revive linter
type APIFakeIntakePayloadsJsonGETResponse struct {
	Payloads []ParsedPayload `json:"payloads"`
}

//nolint:revive // TODO(APL) Fix revive linter
type RouteStat struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

//nolint:revive // TODO(APL) Fix revive linter
type APIFakeIntakeRouteStatsGETResponse struct {
	Routes map[string]RouteStat `json:"routes"`
}

// PARTaskResult captures what the Private Action Runner published for a completed task.
type PARTaskResult struct {
	TaskID       string                 `json:"task_id"`
	Success      bool                   `json:"success"`
	Outputs      map[string]interface{} `json:"outputs,omitempty"`
	ErrorCode    int                    `json:"error_code,omitempty"`
	ErrorDetails string                 `json:"error_details,omitempty"`
}

// ResponseOverride is a hardcoded response for requests to the given endpoint
type ResponseOverride struct {
	Endpoint    string `json:"endpoint"`
	StatusCode  int    `json:"status_code"`
	ContentType string `json:"content_type"`
	Method      string `json:"method"`
	Body        []byte `json:"body"`
}

// RCConfig is a single Remote Config entry exposed via the fakeintake control API.
type RCConfig struct {
	OrgID      string `json:"org_id"`
	Product    string `json:"product"`
	ConfigID   string `json:"config_id"`
	ConfigName string `json:"config_name"`
	Data       []byte `json:"data"`
}

// RCAddConfigRequest is the body accepted by POST /fakeintake/rc/config.
// Data may be raw JSON bytes (preferred) or any JSON value; the server
// re-marshals it for stable storage.
type RCAddConfigRequest struct {
	OrgID      string          `json:"org_id"`
	Product    string          `json:"product"`
	ConfigID   string          `json:"config_id"`
	ConfigName string          `json:"config_name"`
	Data       json.RawMessage `json:"data"`
}

// RCSetExpirationRequest controls the expiry of generated non-root TUF metadata.
type RCSetExpirationRequest struct {
	ExpiresAt time.Time `json:"expires_at"`
}

// RCStats is returned by GET /fakeintake/rc/stats.
type RCStats struct {
	Polls        uint64    `json:"polls"`
	LastPoll     time.Time `json:"last_poll"`
	Version      uint64    `json:"version"`
	ConfigsCount int       `json:"configs_count"`
	KeyID        string    `json:"key_id"`
	PublicKey    string    `json:"public_key"`
	RootJSON     string    `json:"root_json"`
}

// PayloadPage is a bounded raw-history response. NextCursor can be reused to
// poll after HasMore becomes false. Retention and flush can expire a cursor.
type PayloadPage struct {
	Payloads   []Payload `json:"payloads"`
	NextCursor string    `json:"next_cursor"`
	HasMore    bool      `json:"has_more"`
}
