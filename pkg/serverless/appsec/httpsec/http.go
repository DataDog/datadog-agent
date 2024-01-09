// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package httpsec defines is the HTTP instrumentation API and contract for
// AppSec. It defines an abstract representation of HTTP handlers, along with
// helper functions to wrap (aka. instrument) standard net/http handlers.
// HTTP integrations must use this package to enable AppSec features for HTTP,
// which listens to this package's operation events.
package httpsec

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	waf "github.com/DataDog/go-libddwaf/v2"
	json "github.com/json-iterator/go"
)

// Monitorer is the interface type expected by the httpsec invocation
// subprocessor monitoring the given security rules addresses and returning
// the security events that matched.
type Monitorer interface {
	Monitor(addresses map[string]any) *waf.Result
}

// AppSec monitoring context including the full list of monitored HTTP values
// (which must be nullable to know when they were set or not), along with the
// required context to report appsec-related span tags.
type context struct {
	requestSourceIP   string
	requestRoute      *string             // http.route
	requestClientIP   *string             // http.client_ip
	requestRawURI     *string             // server.request.uri.raw
	requestHeaders    map[string][]string // server.request.headers.no_cookies
	requestCookies    map[string][]string // server.request.cookies
	requestQuery      map[string][]string // server.request.query
	requestPathParams map[string]string   // server.request.path_params
	requestBody       interface{}         // server.request.body
	responseStatus    *string             // server.response.status
}

// makeContext creates a http monitoring context out of the provided arguments.
func makeContext(ctx *context, route, path *string, headers, queryParams map[string][]string, pathParams map[string]string, sourceIP string, rawBody *string, isBodyBase64 bool) {
	headers, rawCookies := filterHeaders(headers)
	cookies := parseCookies(rawCookies)
	body := parseBody(headers, rawBody, isBodyBase64)
	*ctx = context{
		requestRoute:      route,
		requestSourceIP:   sourceIP,
		requestRawURI:     path,
		requestHeaders:    headers,
		requestCookies:    cookies,
		requestQuery:      queryParams,
		requestPathParams: pathParams,
		requestBody:       body,
	}
}

// parseBody attempts to parse the payload found in rawBody according to the presentation headers. Returns nil if the
// request body could not be parsed (either due to an error, or because no suitable parsing strategy is implemented).
func parseBody(headers map[string][]string, rawBody *string, isBodyBase64 bool) any {
	if rawBody == nil {
		return nil
	}

	bodyDecoded := *rawBody
	if isBodyBase64 {
		rawBodyDecoded, err := base64.StdEncoding.DecodeString(bodyDecoded)
		if err != nil {
			log.Errorf("cannot decode '%s' from base64: %v", bodyDecoded, err)
			return nil
		}

		bodyDecoded = string(rawBodyDecoded)
	}

	// textproto.MIMEHeader normalizes the header names, so we don't have to worry about text case.
	mimeHeaders := make(textproto.MIMEHeader, len(headers))
	for key, values := range headers {
		for _, value := range values {
			mimeHeaders.Add(key, value)
		}
	}

	result, err := tryParseBody(mimeHeaders, bodyDecoded)
	if err != nil {
		log.Warnf("unable to parse request body: %v", err)
		return nil
	}

	return result
}

// / tryParseBody attempts to parse the raw data in raw according to the headers. Returns an error if parsing
// / fails, and a nil body if no parsing strategy was found.
func tryParseBody(headers textproto.MIMEHeader, raw string) (body any, err error) {
	var mediaType string
	var params map[string]string

	if value := headers.Get("Content-Type"); value == "" {
		return nil, nil
	} else { //nolint:revive // TODO(ASM) Fix revive linter
		mt, p, err := mime.ParseMediaType(value)
		if err != nil {
			return nil, err
		}
		mediaType = mt
		params = p
	}

	switch mediaType {
	case "application/json", "application/vnd.api+json":
		if err := json.Unmarshal([]byte(raw), &body); err != nil {
			return nil, err
		}
		return body, nil

	case "application/x-www-form-urlencoded":
		values, err := url.ParseQuery(raw)
		if err != nil {
			return nil, err
		}
		return map[string][]string(values), nil

	case "application/xml", "text/xml":
		var value xmlMap
		if err := xml.Unmarshal([]byte(raw), &value); err != nil {
			return nil, err
		}
		// Unwrap the value to avoid surfacing our implementation details out
		return map[string]any(value), nil

	case "multipart/form-data":
		boundary, ok := params["boundary"]
		if !ok {
			return nil, fmt.Errorf("cannot parse a multipart/form-data payload without a boundary")
		}
		mr := multipart.NewReader(strings.NewReader(raw), boundary)

		data := make(map[string]any)
		for {
			part, err := mr.NextPart()
			if err != nil {
				if err == io.EOF {
					break
				}
				return nil, err
			}
			defer part.Close()

			partData := make(map[string]any, 2)

			partRawBody, err := io.ReadAll(part)
			if err != nil {
				return nil, err
			}
			partBody, err := tryParseBody(map[string][]string(part.Header), string(partRawBody))
			if err != nil {
				log.Debugf("failed to parse multipart/form-data part: %v", err)
				partData["data"] = nil
			} else {
				partData["data"] = partBody
			}

			if filename := part.FileName(); filename != "" {
				partData["filename"] = filename
			}

			data[part.FormName()] = partData
		}
		return data, nil

	case "text/plain":
		return raw, nil

	default:
		return nil, nil
	}
}

func (c *context) toAddresses() map[string]interface{} {
	addr := make(map[string]interface{})
	if c.requestClientIP != nil {
		addr["http.client_ip"] = *c.requestClientIP
	}
	if c.requestRawURI != nil {
		addr["server.request.uri.raw"] = *c.requestRawURI
	}
	if c.requestHeaders != nil {
		addr["server.request.headers.no_cookies"] = c.requestHeaders
	}
	if c.requestCookies != nil {
		addr["server.request.cookies"] = c.requestCookies
	}
	if c.requestQuery != nil {
		addr["server.request.query"] = c.requestQuery
	}
	if c.requestPathParams != nil {
		addr["server.request.path_params"] = c.requestPathParams
	}
	if c.requestBody != nil {
		addr["server.request.body"] = c.requestBody
	}
	if c.responseStatus != nil {
		addr["server.response.status"] = c.responseStatus
	}
	return addr
}

// FilterHeaders copies the given map and filters out the cookie entry. The
// resulting map of filtered headers is returned, along with the removed cookie
// entry if any. Note that the keys of the returned map of headers have been
// lower-cased as expected by Datadog's security monitoring rules - accessing
// them using http.(Header).Get(), which expects the MIME header canonical
// format, doesn't work.
func filterHeaders(reqHeaders map[string][]string) (headers map[string][]string, rawCookies []string) {
	if len(reqHeaders) == 0 {
		return nil, nil
	}
	// Walk the map of request headers and filter the cookies out if any
	headers = make(map[string][]string, len(reqHeaders))
	for k, v := range reqHeaders {
		k := strings.ToLower(k)
		if k == "cookie" {
			// Do not include cookies in the request headers
			rawCookies = v
		}
		headers[k] = v
	}
	if len(headers) == 0 {
		headers = nil // avoid returning an empty map
	}
	return headers, rawCookies
}

// ParseCookies returns the parsed cookies as a map of the cookie names to their
// value. Cookies defined more than once have multiple values in their map
// entry.
func parseCookies(rawCookies []string) map[string][]string {
	// net.http doesn't expose its cookie-parsing function, so we are using the
	// http.(*Request).Cookies method instead which reads the request headers.
	r := http.Request{Header: map[string][]string{"Cookie": rawCookies}}
	parsed := r.Cookies()
	if len(parsed) == 0 {
		return nil
	}
	cookies := make(map[string][]string, len(parsed))
	for _, c := range parsed {
		cookies[c.Name] = append(cookies[c.Name], c.Value)
	}
	return cookies
}

// Helper function to convert a single-value map of event values into a
// multi-value one.
func toMultiValueMap(m map[string]string) map[string][]string {
	l := len(m)
	if l == 0 {
		return nil
	}
	res := make(map[string][]string, l)
	for k, v := range m {
		res[k] = []string{v}
	}
	return res
}

// xmlMap is used to parse XML documents into a schema-agnostic format (essentially, a `map[string]any`).
type xmlMap map[string]any

// UnmarshalXML implements custom parsing from XML documents into a map-based generic format, because encoding/xml does
// not provide a built-in unmarshal to map (any data that does not fit an `xml` tagged field, or that does not fit the
// shape of that field, is silently ignored).
func (m *xmlMap) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var children []any
out:
	for {
		token, err := d.Token()
		if err != nil {
			return err
		}

		switch token := token.(type) {
		case xml.StartElement:
			var child xmlMap
			if err := child.UnmarshalXML(d, token); err != nil {
				return err
			}
			// Unwrap so we don't surface our implementation details out to the world...
			children = append(children, map[string]any(child))
		case xml.EndElement:
			if token.Name.Local != start.Name.Local || token.Name.Space != start.Name.Space {
				return fmt.Errorf("unexpected end of element %s", token.Name.Local)
			}
			break out
		case xml.CharData:
			str := strings.TrimSpace(string(token))
			if str != "" {
				children = append(children, str)
			}
		case xml.Comment:
			str := strings.TrimSpace(string(token))
			children = append(children, map[string]string{"#": str})
		case xml.ProcInst:
			children = append(children, map[string]any{"?": map[string]string{
				"target":      token.Target,
				"instruction": string(token.Inst),
			}})
		case xml.Directive:
			children = append(children, map[string]any{"!": string(token)})
		default:
			return fmt.Errorf("not implemented: %T", token)
		}
	}

	element := map[string]any{"children": children}
	if start.Name.Space != "" {
		element[":ns"] = start.Name.Space
	}
	for _, attr := range start.Attr {
		prefix := ""
		if attr.Name.Space != "" {
			prefix = attr.Name.Space + ":"
		}
		element[fmt.Sprintf("@%s%s", prefix, attr.Name.Local)] = attr.Value
	}

	*m = xmlMap{start.Name.Local: element}
	return nil
}
