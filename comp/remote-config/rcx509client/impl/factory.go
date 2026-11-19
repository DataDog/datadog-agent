// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !test

package rcx509clientimpl

import (
	"errors"
	"fmt"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	rcx509client "github.com/DataDog/datadog-agent/comp/remote-config/rcx509client/def"
	"github.com/DataDog/libdd-rc/ffi-hosts/go/rcx509"
)

const debugServicePingURI = "rc.x509.magic_tunnel.remote_config.v1.DebugService/Ping"

// NewClientFactory returns the production adapter for the public libdd-rc Go
// client. Keeping this boundary injectable lets the component test lifecycle
// behavior without initializing the native x509 subsystem.
func NewClientFactory(logger log.Component) rcx509client.ClientFactory {
	return func(config rcx509client.ClientConfig) (rcx509client.Client, error) {
		client, err := rcx509.NewClient(
			config.URL,
			config.AppName,
			config.Version,
			rcx509.WithAPIKey(config.APIKey),
			rcx509.WithHTTPClient(config.HTTPClient),
		)
		if err != nil {
			return nil, err
		}

		if config.DebugPingEnabled {
			if err := client.RegisterHandler(debugServicePingURI, newDebugPingHandler(logger)); err != nil {
				_ = client.Close()
				return nil, fmt.Errorf("register x509 debug ping handler: %w", err)
			}
			logger.Info("remote config x509 debug ping handler enabled for manual staging validation")
		}

		return hostTagsClient{client}, nil
	}
}

// errHostTagsUnsupported is returned when the pinned libdd-rc release does not
// support host tags yet.
var errHostTagsUnsupported = errors.New("remote config x509 client does not support host tags in this libdd-rc release")

// hostTagsClient adds SetHostTags to the pinned rcx509.Client.
//
// PoC: libdd-rc v0.1.0 has no Client.SetHostTags. This checks for it at run
// time so the Agent builds against today's pin and picks it up after a
// re-pin. Remove it once the pinned release has SetHostTags.
type hostTagsClient struct {
	*rcx509.Client
}

func (c hostTagsClient) SetHostTags(tags []string) error {
	setter, ok := any(c.Client).(interface{ SetHostTags([]string) error })
	if !ok {
		return errHostTagsUnsupported
	}
	return setter.SetHostTags(tags)
}
