// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !aix && !freebsd && !solaris

package datadogextension

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

type diSender struct {
	client        *http.Client
	key, endpoint string
}

var diSitePattern = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`)

func validateDIConfig(c *Config) error {
	if strings.TrimSpace(string(c.Datadog.API.Key)) == "" || !diSitePattern.MatchString(c.Datadog.API.Site) {
		return errors.New("DI requires the extension's api.key and valid api.site")
	}
	backend := c.Datadog.ClientConfig
	if backend.TLS.Insecure || backend.TLS.InsecureSkipVerify || backend.Auth.HasValue() || len(backend.Headers) > 0 || len(backend.Middlewares) > 0 || backend.Compression.IsCompressed() {
		return errors.New("DI requires verified TLS and api.key authentication without custom headers, middleware or compression")
	}
	return nil
}

// ConsumeDIEvents never forwards DI records to the ordinary logs pipeline.
func (e *datadogExtension) ConsumeDIEvents(ctx context.Context, logs plog.Logs) error {
	if e.di == nil || e.di.client == nil {
		return consumererror.NewPermanent(errors.New("Datadog extension DI delivery is disabled"))
	}
	type request struct {
		body              []byte
		contentType, tags string
	}
	requests := make([]request, 0, logs.LogRecordCount())
	for i := 0; i < logs.ResourceLogs().Len(); i++ {
		rl := logs.ResourceLogs().At(i)
		for j := 0; j < rl.ScopeLogs().Len(); j++ {
			sl := rl.ScopeLogs().At(j)
			for k := 0; k < sl.LogRecords().Len(); k++ {
				r := sl.LogRecords().At(k)
				body, contentType, err := encodeDIEvent(r)
				if err != nil {
					return consumererror.NewPermanent(err)
				}
				tags, _ := r.Attributes().Get("datadog.di.tags")
				requests = append(requests, request{body, contentType, tags.Str()})
			}
		}
	}
	for _, r := range requests {
		if err := e.di.send(ctx, r.body, r.contentType, r.tags); err != nil {
			return err
		}
	}
	return nil
}

func encodeDIEvent(record plog.LogRecord) ([]byte, string, error) {
	version, ok := record.Attributes().Get("datadog.di.schema_version")
	if !ok || version.Type() != pcommon.ValueTypeInt || version.Int() != 1 {
		return nil, "", errors.New("unsupported DI event schema")
	}
	payload, ok := record.Attributes().Get("datadog.di.payload")
	if !ok || payload.Type() != pcommon.ValueTypeStr || !json.Valid([]byte(payload.Str())) || !strings.HasPrefix(strings.TrimSpace(payload.Str()), "{") {
		return nil, "", errors.New("DI event requires a JSON object payload")
	}
	switch record.EventName() {
	case "dd.debugger.snapshot", "dd.debugger.log":
		return []byte("[" + payload.Str() + "]"), "application/json", nil
	case "dd.debugger.diagnostic":
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", `form-data; name="event"; filename="event.json"`)
		header.Set("Content-Type", "application/json")
		part, err := writer.CreatePart(header)
		if err != nil {
			return nil, "", err
		}
		if _, err := io.WriteString(part, "["+payload.Str()+"]"); err != nil {
			return nil, "", err
		}
		if err := writer.Close(); err != nil {
			return nil, "", err
		}
		return body.Bytes(), writer.FormDataContentType(), nil
	case "dd.debugger.symbols":
		return encodeSymbols([]byte(payload.Str()))
	default:
		return nil, "", errors.New("unsupported DI event name")
	}
}

func encodeSymbols(payload []byte) ([]byte, string, error) {
	var envelope struct {
		Event   map[string]json.RawMessage `json:"event"`
		Symbols json.RawMessage            `json:"symbols"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, "", err
	}
	if len(envelope.Event) == 0 || len(envelope.Symbols) == 0 || !strings.HasPrefix(strings.TrimSpace(string(envelope.Symbols)), "{") {
		return nil, "", errors.New("symbol event requires metadata and a symbols object")
	}
	var compressed bytes.Buffer
	zip := gzip.NewWriter(&compressed)
	if _, err := zip.Write(envelope.Symbols); err != nil {
		return nil, "", err
	}
	if err := zip.Close(); err != nil {
		return nil, "", err
	}
	envelope.Event["attachmentSize"] = json.RawMessage(strconv.Itoa(compressed.Len()))
	event, err := json.Marshal(envelope.Event)
	if err != nil {
		return nil, "", err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range []struct {
		name, file, contentType string
		data                    []byte
	}{
		{"event", "event.json", "application/json", event},
		{"file", "symbols.json.gz", "application/gzip", compressed.Bytes()},
	} {
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, part.name, part.file))
		header.Set("Content-Type", part.contentType)
		w, err := writer.CreatePart(header)
		if err != nil {
			return nil, "", err
		}
		if _, err := w.Write(part.data); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}

func (s *diSender) send(ctx context.Context, payload []byte, contentType, tags string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint+"?"+url.Values{"ddtags": {tags}}.Encode(), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("DD-API-KEY", s.key)
	req.Header.Set("DD-EVP-ORIGIN", "agent-debugger")
	req.Header.Set("Content-Type", contentType)
	response, err := s.client.Do(req)
	if err != nil {
		return errors.New("DI intake transport failed")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("DI intake returned HTTP %d", response.StatusCode)
	}
	return nil
}
