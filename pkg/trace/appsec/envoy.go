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

	"github.com/DataDog/datadog-agent/pkg/trace/log"
	waf "github.com/DataDog/go-libddwaf"
	//envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_service_auth_v3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

type server struct {
	wafHandle *waf.Handle
}

// Static assertion that &server{} implements the expected Go interface
var _ envoy_service_auth_v3.AuthorizationServer = &server{}

// NewEnvoyAuthorizationServer creates a new envoy authorization server.
func NewEnvoyAuthorizationServer(wafHandle *waf.Handle) envoy_service_auth_v3.AuthorizationServer {
	return &server{
		wafHandle: wafHandle,
	}
}

func reqToClientIp(req *envoy_service_auth_v3.CheckRequest) string {
	sockAddr := req.Attributes.Source.Address.GetSocketAddress()
	if sockAddr == nil {
		log.Warnf("appsec envoy authorization api: unsupported source: %v", req.Attributes.Source)
		return ""
	}
	// TODO support for client IP detection
	peerIp := sockAddr.Address
	return peerIp
}

func reqToAddrs(req *envoy_service_auth_v3.CheckRequest) map[string]interface{} {
	httpReq := req.Attributes.Request.Http
	httpPathIdx := strings.Index(httpReq.Path, "?")
	if httpPathIdx == -1 {
		httpPathIdx = len(httpReq.Path)
	}
	httpUrl := url.URL{
		Scheme:   httpReq.Scheme,
		Host:     httpReq.Host,
		Path:     httpReq.Path[:httpPathIdx],
		RawQuery: httpReq.Path[httpPathIdx:],
		Fragment: httpReq.Fragment,
	}
	httpHeaders := map[string]string{}
	for key, val := range httpReq.Headers {
		if len(key) == 0 || key[0] == ':' || key == "cookie" {
			continue
		}
		httpHeaders[key] = val
	}
	addresses := map[string]interface{}{
		"http.client_ip":                    reqToClientIp(req),
		"server.request.method":             httpReq.Method,
		"server.request.uri.raw":            httpUrl.String(),
		"server.request.headers.no_cookies": httpHeaders,
	}
	return addresses
}

// Check implements authorization's Check interface which performs authorization check based on the
// attributes associated with the incoming request.
func (s *server) Check(ctx context.Context, req *envoy_service_auth_v3.CheckRequest) (*envoy_service_auth_v3.CheckResponse, error) {
	okResponse := &envoy_service_auth_v3.CheckResponse{
		Status: &status.Status{
			Code: int32(code.Code_OK),
		},
	}

	wafCtx := waf.NewContext(s.wafHandle)
	if wafCtx == nil {
		// The WAF handle was released
		return okResponse, nil
	}
	defer wafCtx.Close()

	addresses := reqToAddrs(req)
	log.Debugf("appsec: envoy auth api: running the security rules against %v", addresses)
	matches, actions, err := wafCtx.Run(addresses, defaultWAFTimeout)
	if err != nil && err != waf.ErrTimeout {
		log.Errorf("appsec: unexpected waf execution error: %v", err)
	}
	log.Debugf("appsec: envoy auth api: matches=%s actions=%v", string(matches), actions)

	if len(matches) > 0 {
		meta := &structpb.Struct{Fields: make(map[string]*structpb.Value)}
		meta.Fields["appsec"] = structpb.NewStringValue(fmt.Sprintf("{\"event\":true,\"triggers\":%s}", matches))
		meta.Fields["origin"] = structpb.NewStringValue("appsec")
		okResponse.DynamicMetadata = meta
		//okResponse.HttpResponse.OkHttpResponse = envoy_service_auth_v3.OkHttpResponse{Headers: []*envoy_config_core_v3.HeaderValueOption{
		//	{Header: envoy_config_core_v3.HeaderValue{Key: "x-datadog-sampling-priority", Value: "1"}},
		//}}
	}

	return okResponse, nil
}
