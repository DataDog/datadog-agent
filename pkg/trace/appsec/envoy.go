// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/trace/log"
	waf "github.com/DataDog/go-libddwaf"
	envoy_service_auth_v3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/genproto/googleapis/rpc/status"
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

	// TODO: create all the sec addresses out of CheckRequest
	httpReq := req.Attributes.Request.Http
	addresses := map[string]interface{}{
		"server.request.headers.no_cookies": httpReq.Headers,
	}

	log.Debug("appsec: envoy auth api: running the security rules against %v", addresses)
	matches, actions, err := wafCtx.Run(addresses, defaultWAFTimeout)
	if err != nil && err != waf.ErrTimeout {
		log.Errorf("appsec: unexpected waf execution error: %v", err)
	}
	log.Debug("appsec: envoy auth api: matches=%s actions=%v", string(matches), actions)

	// TODO: create appsec span tags

	return okResponse, nil
}
