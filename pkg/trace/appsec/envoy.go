// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	waf "github.com/DataDog/go-libddwaf"
	envoy_service_auth_v3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

type server struct {
	wafManager *Manager
}

// Static assertion that &server{} implements the expected Go interface
var _ envoy_service_auth_v3.AuthorizationServer = &server{}

// NewEnvoyAuthorizationServer creates a new envoy authorization server.
func NewEnvoyAuthorizationServer(wafManager *Manager) envoy_service_auth_v3.AuthorizationServer {
	return &server{
		wafManager: wafManager,
	}
}

// Return the first global client ip address based on the request TCP source ip
// and http headers. The result is an empty string if no global IP address was
// found.
func reqToClientIp(req *envoy_service_auth_v3.CheckRequest) string {
	sockAddr := req.Attributes.Source.Address.GetSocketAddress()
	if sockAddr == nil {
		log.Warnf("appsec envoy authorization api: unsupported source: %v", req.Attributes.Source)
		return ""
	}
	peerIp := sockAddr.Address
	headers := req.Attributes.Request.Http.Headers
	clientIp, _ := makeClientIPTags(peerIp, headers)
	return clientIp
}

func reqToAddrs(req *envoy_service_auth_v3.CheckRequest) map[string]interface{} {
	httpReq := req.Attributes.Request.Http
	httpUrl := string(httpReq.Path)
	if httpReq.Fragment != "" {
		httpUrl += "#"
		httpUrl += httpReq.Fragment
	}
	httpQuery := map[string][]string{}
	httpPathIdx := strings.Index(httpReq.Path, "?")
	if httpPathIdx != -1 {
		httpQuery, _ = url.ParseQuery(httpReq.Path[httpPathIdx+1:])
	}
	httpHeaders := map[string]string{}
	for key, val := range httpReq.Headers {
		if len(key) == 0 || key[0] == ':' || key == "cookie" {
			continue
		}
		httpHeaders[key] = val
	}
	addresses := map[string]interface{}{
		"server.request.method":             httpReq.Method,
		"server.request.uri.raw":            httpUrl,
		"server.request.query":              httpQuery,
		"server.request.headers.no_cookies": httpHeaders,
	}
	if clientIP := reqToClientIp(req); clientIP != "" {
		addresses["http.client_ip"] = clientIP
	}
	return addresses
}

func shouldBlock(actions []string) bool {
	for _, action := range actions {
		if action == "block" {
			return true
		}
	}
	return false
}

func attachAppSecMetadata(resp *envoy_service_auth_v3.CheckResponse, clientIp string, matches []byte, blocked bool) {
	if len(matches) == 2 && matches[0] == '[' && matches[1] == ']' {
		return
	}
	meta := resp.DynamicMetadata
	if meta == nil {
		meta = &structpb.Struct{}
		resp.DynamicMetadata = meta
	}
	fields := meta.Fields
	if fields == nil {
		fields = make(map[string]*structpb.Value)
		meta.Fields = fields
	}
	// field to be copied into _dd.appsec.json by envoy tracer
	rawBlocked := ""
	if blocked {
		rawBlocked = "\"blocked\":true,"
		fields["appsec.blocked"] = structpb.NewStringValue("true")
	}
	if clientIp != "" {
		fields["http.client_ip"] = structpb.NewStringValue(clientIp)
	}
	fields["appsec.event"] = structpb.NewStringValue("true")
	fields["appsec"] = structpb.NewStringValue(fmt.Sprintf("{\"event\":true,%s\"triggers\":%s}", rawBlocked, matches))
}

// Check implements authorization's Check interface which performs authorization check based on the
// attributes associated with the incoming request.
func (s *server) Check(ctx context.Context, req *envoy_service_auth_v3.CheckRequest) (*envoy_service_auth_v3.CheckResponse, error) {
	resp := &envoy_service_auth_v3.CheckResponse{
		Status: &status.Status{
			Code: int32(code.Code_OK),
		},
	}

	serviceName, ok := req.Attributes.ContextExtensions["service_name"]
	if !ok {
		log.Error("appsec: cannot check request because service name is missing")
		return resp, nil
	}
	env, ok := req.Attributes.ContextExtensions["env"]
	if !ok {
		env = config.GetTraceAgentDefaultEnv()
	}

	wafCtx := s.wafManager.GetWafContextForService(serviceName, env)
	if wafCtx == nil {
		// The WAF handle was released
		return resp, nil
	}
	defer wafCtx.Close()

	addresses := reqToAddrs(req)
	log.Debugf("appsec: envoy auth api: running the security rules against %v", addresses)
	matches, actions, err := wafCtx.Run(addresses, defaultWAFTimeout)
	if err != nil && err != waf.ErrTimeout {
		log.Errorf("appsec: unexpected waf execution error: %v", err)
	}
	log.Debugf("appsec: envoy auth api: matches=%v actions=%v", matches, actions)
	block := shouldBlock(actions)
	if block {
		resp.Status.Code = int32(code.Code_PERMISSION_DENIED)
		resp.HttpResponse = &envoy_service_auth_v3.CheckResponse_DeniedResponse{
			&envoy_service_auth_v3.DeniedHttpResponse{
				Status: &envoy_type_v3.HttpStatus{
					Code: 403,
				},
			},
		}
	}
	attachAppSecMetadata(resp, addresses["http.client_ip"].(string), matches, block)
	return resp, nil
}
