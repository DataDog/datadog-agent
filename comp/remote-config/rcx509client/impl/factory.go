// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcx509clientimpl

import (
	"fmt"
	"os"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	rcx509client "github.com/DataDog/datadog-agent/comp/remote-config/rcx509client/def"
	"github.com/DataDog/libdd-rc/ffi-hosts/go/rcx509"
)

// NewClientFactory returns the production adapter for the public libdd-rc Go
// client. Keeping this boundary injectable lets the component test lifecycle
// behavior without initializing the native x509 subsystem.
func NewClientFactory(logger log.Component) rcx509client.ClientFactory {
	return func(config rcx509client.ClientConfig) (rcx509client.Client, error) {
		if config.DebugPingEnabled {
			// This installs libdd-rc's process-wide tracing sink. Keep it tied to
			// the manual validation flag; the default info filter exposes the
			// verified connection ID without logging request payloads.
			if err := rcx509.EnableLogSink(os.Stderr); err != nil {
				return nil, fmt.Errorf("enable x509 client log sink: %w", err)
			}
		}

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

		return client, nil
	}
}
