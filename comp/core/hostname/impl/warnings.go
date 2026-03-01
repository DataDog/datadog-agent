// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package hostnameimpl

import (
	"context"
	"os"
	"runtime"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// isHostnameCanonicalForIntake returns true if the intake will use the hostname as the canonical hostname.
// The intake prefers the EC2 instance ID over an EC2 default hostname.
func isHostnameCanonicalForIntake(ctx context.Context, hostname string) bool {
	if ec2.IsDefaultHostnameForIntake(hostname) {
		_, err := ec2GetInstanceID(ctx)
		return err != nil
	}
	return true
}

// warnIfNotCanonicalHostname logs a warning if the configured hostname will not be used as
// the in-app canonical hostname (because the intake prefers the EC2 instance ID).
func warnIfNotCanonicalHostname(ctx context.Context, cfg pkgconfigmodel.Reader, hostname string) {
	if !isHostnameCanonicalForIntake(ctx, hostname) && !cfg.GetBool("hostname_force_config_as_canonical") {
		log.Warnf(
			"Hostname '%s' defined in configuration will not be used as the in-app hostname. "+
				"For more information: https://dtdg.co/agent-hostname-force-config-as-canonical",
			hostname,
		)
	}
}

// warnAboutFQDN logs a deprecation notice when the FQDN differs from the OS hostname
// and the `hostname_fqdn` flag is not enabled.
func warnAboutFQDN(_ context.Context, cfg pkgconfigmodel.Reader, hostname string) {
	fqdn, _ := fqdnHostname()
	if fqdn == "" {
		return
	}

	h, err := os.Hostname()
	if err != nil {
		return
	}

	// Only warn when not using FQDN but the FQDN would differ from the resolved hostname.
	if !cfg.GetBool("hostname_fqdn") && hostname == h && h != fqdn {
		if runtime.GOOS != "windows" {
			// REMOVEME: This should be removed when the default `hostname_fqdn` is set to true
			log.Warnf("DEPRECATION NOTICE: The agent resolved your hostname as '%s'. However in a future version, it will be resolved as '%s' by default. To enable the future behavior, please enable the `hostname_fqdn` flag in the configuration. For more information: https://dtdg.co/flag-hostname-fqdn", h, fqdn)
		} else {
			log.Warnf("The agent resolved your hostname as '%s', and will be reported this way to maintain compatibility with version 5. To enable reporting as '%s', please enable the `hostname_fqdn` flag in the configuration. For more information: https://dtdg.co/flag-hostname-fqdn", h, fqdn)
		}
	}
}
