// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path"
	"strings"

	"github.com/moby/moby/api/types/system"
	"github.com/moby/moby/client"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// safeInfo returns the Docker daemon's /info response, working around daemons
// that emit invalid CIDRs in DefaultAddressPools[].Base. The moby v29 SDK
// decodes Base into a netip.Prefix, whose UnmarshalText is strict and rejects
// such values, which would fail the entire /info JSON decode and break every
// caller (init probe, hostname provider, host tags, host metadata, storage
// stats).
//
// Strategy: try the SDK's Info() first. If it fails with the SDK's JSON-decode
// wrapper, retry with a raw HTTP request that decodes into a mirror struct
// where DefaultAddressPools is captured as json.RawMessage, shadowing the
// strict field and letting the rest of /info parse normally.
func safeInfo(ctx context.Context, cli *client.Client) (system.Info, error) {
	result, err := cli.Info(ctx, client.InfoOptions{})
	if err == nil {
		return result.Info, nil
	}

	// The moby client wraps JSON-decode failures of /info with this prefix
	// (see github.com/moby/moby/client/system_info.go). Network, HTTP-status
	// and other connection errors do not, and the tolerant fallback would not
	// help in those cases — propagate the original error.
	if !strings.Contains(err.Error(), "Error reading remote info") {
		return system.Info{}, err
	}

	log.Debugf("Docker /info decode failed (%v); retrying with tolerant decoder", err)
	info, fallbackErr := tolerantInfo(ctx, cli)
	if fallbackErr != nil {
		return system.Info{}, errors.Join(err, fmt.Errorf("tolerant /info fallback: %w", fallbackErr))
	}
	return info, nil
}

// tolerantInfo reissues GET /info through the moby client's dialer and decodes
// the response into a struct that shadows DefaultAddressPools with a
// json.RawMessage. This bypasses the strict netip.Prefix decoding of the typed
// field while leaving the rest of system.Info populated.
func tolerantInfo(ctx context.Context, cli *client.Client) (system.Info, error) {
	httpClient := &http.Client{
		Transport: &http.Transport{
			// One-shot client: no benefit from keep-alive, and DisableKeepAlives
			// ensures the dialed connection is closed when the response body
			// is, so the unreferenced transport does not retain FDs.
			DisableKeepAlives: true,
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return cli.Dialer()(ctx)
			},
		},
	}

	// The dialer takes care of reaching the daemon (unix, npipe, tcp, tcp+tls).
	// For TCP daemons, preserve the configured host and base path so reverse
	// proxies relying on Host-header or path routing reach the same backend
	// as the SDK does. For unix/npipe, the SDK uses DummyHost — match it.
	// Use the "http" scheme even for tls-fronted daemons: the dialer returns
	// an already-TLS-encrypted connection, and the http transport writes plain
	// HTTP bytes over it.
	reqHost := client.DummyHost
	basePath := ""
	if hostURL, err := client.ParseHostURL(cli.DaemonHost()); err == nil {
		if hostURL.Scheme == "tcp" {
			reqHost = hostURL.Host
		}
		basePath = hostURL.Path
	}
	// Match the SDK's path construction so reverse proxies routing on
	// /vX.Y/info don't reject the fallback (see moby client.getAPIPath).
	apiPath := "/info"
	if v := cli.ClientVersion(); v != "" {
		apiPath = "/v" + strings.TrimPrefix(v, "v") + apiPath
	}
	url := "http://" + reqHost + path.Join("/", basePath, apiPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return system.Info{}, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return system.Info{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return system.Info{}, fmt.Errorf("unexpected status %d from /info", resp.StatusCode)
	}

	// Outer DefaultAddressPools (json.RawMessage) shadows the promoted field
	// from the embedded system.Info: encoding/json routes the JSON value to
	// the less-nested field, leaving system.Info.DefaultAddressPools at its
	// zero value. All other fields decode through the embedded struct.
	var tolerant struct {
		system.Info
		DefaultAddressPools json.RawMessage `json:"DefaultAddressPools"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tolerant); err != nil {
		return system.Info{}, fmt.Errorf("tolerant decode of /info failed: %w", err)
	}
	return tolerant.Info, nil
}
