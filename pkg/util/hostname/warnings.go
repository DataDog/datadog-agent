// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostname

import (
	"context"
	"os"
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// isHostnameCanonicalForIntake returns true if the intake will use the hostname as canonical hostname.
func isHostnameCanonicalForIntake(ctx context.Context, hostname string) bool {
	// Intake uses instance id for ec2 default hostname except for Windows.
	if ec2.IsDefaultHostnameForIntake(hostname) {
		_, err := ec2.GetInstanceID(ctx)
		return err != nil
	}
	return true
}

func warnIfNotCanonicalHostname(ctx context.Context, hostname string) {
	if !isHostnameCanonicalForIntake(ctx, hostname) && !config.Datadog().GetBool("hostname_force_config_as_canonical") {
		log.Warnf(
			"Hostname '%s' defined in configuration will not be used as the in-app hostname. "+
				"For more information: https://dtdg.co/agent-hostname-force-config-as-canonical",
			hostname,
		)
	}
}

func warnAboutFQDN(ctx context.Context, hostname string) {
	fqdn, _ := fromFQDN(ctx, "")
	if fqdn == "" {
		return
	}

	h, err := os.Hostname()
	if err != nil {
		return
	}

	// We have a FQDN that does not match to the resolved hostname, and the configuration
	// field `hostname_fqdn` isn't set -> we display a warning message about
	// the future behavior
	if !config.Datadog().GetBool("hostname_fqdn") && hostname == h && h != fqdn {
		if runtime.GOOS != "windows" {
			// REMOVEME: This should be removed when the default `hostname_fqdn` is set to true
			log.Warnf("DEPRECATION NOTICE: The agent resolved your hostname as '%s'. However in a future version, it will be resolved as '%s' by default. To enable the future behavior, please enable the `hostname_fqdn` flag in the configuration. For more information: https://dtdg.co/flag-hostname-fqdn", h, fqdn)
		} else { // OS is Windows
			log.Warnf("The agent resolved your hostname as '%s', and will be reported this way to maintain compatibility with version 5. To enable reporting as '%s', please enable the `hostname_fqdn` flag in the configuration. For more information: https://dtdg.co/flag-hostname-fqdn", h, fqdn)
		}
	}
}
