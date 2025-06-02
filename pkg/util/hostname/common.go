// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostname provides utilities to detect the hostname of the host.
package hostname

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/azure"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/gce"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// for testing purposes
var (
	isFargateInstance                = fargate.IsFargateInstance
	ec2GetInstanceID                 = ec2.GetInstanceID
	ec2GetLegacyResolutionInstanceID = ec2.GetLegacyResolutionInstanceID
	isContainerized                  = env.IsContainerized //nolint:unused
	gceGetHostname                   = gce.GetHostname
	azureGetHostname                 = azure.GetHostname
	osHostname                       = os.Hostname
	fqdnHostname                     = getSystemFQDN
	osHostnameUsable                 = isOSHostnameUsable
)

// Data contains hostname and the hostname provider
type Data = hostnameinterface.Data

func fromConfig(ctx context.Context, _ string) (string, error) {
	configName := pkgconfigsetup.Datadog().GetString("hostname")
	err := validate.ValidHostname(configName)
	if err != nil {
		return "", err
	}
	warnIfNotCanonicalHostname(ctx, configName)
	return configName, nil
}

func fromHostnameFile(ctx context.Context, _ string) (string, error) {
	// Try `hostname_file` config option next
	hostnameFilepath := pkgconfigsetup.Datadog().GetString("hostname_file")
	if hostnameFilepath == "" {
		return "", fmt.Errorf("'hostname_file' configuration is not enabled")
	}

	fileContent, err := os.ReadFile(hostnameFilepath)
	if err != nil {
		return "", fmt.Errorf("Could not read hostname from %s: %v", hostnameFilepath, err)
	}

	hostname := strings.TrimSpace(string(fileContent))

	err = validate.ValidHostname(hostname)
	if err != nil {
		return "", err
	}
	warnIfNotCanonicalHostname(ctx, hostname)
	return hostname, nil
}

func fromFargate(_ context.Context, _ string) (string, error) {
	// If we're running on fargate we strip the hostname
	if isFargateInstance() {
		return "", nil
	}
	return "", fmt.Errorf("agent is not runnning on Fargate")
}

func fromGCE(ctx context.Context, _ string) (string, error) {
	return gceGetHostname(ctx)
}

func fromAzure(ctx context.Context, _ string) (string, error) {
	return azureGetHostname(ctx)
}

func fromFQDN(ctx context.Context, _ string) (string, error) {
	if !osHostnameUsable(ctx) {
		return "", fmt.Errorf("FQDN hostname is not usable")
	}

	if pkgconfigsetup.Datadog().GetBool("hostname_fqdn") {
		fqdn, err := fqdnHostname()
		if err == nil {
			return fqdn, nil
		}
		return "", fmt.Errorf("Unable to get FQDN from system: %s", err)
	}
	return "", fmt.Errorf("'hostname_fqdn' configuration is not enabled")
}

func fromOS(ctx context.Context, currentHostname string) (string, error) {
	if osHostnameUsable(ctx) {
		if currentHostname == "" {
			return osHostname()
		}
		return "", fmt.Errorf("Skipping OS hostname as a previous provider found a valid hostname")
	}
	return "", fmt.Errorf("OS hostname is not usable")
}

func getValidEC2Hostname(ctx context.Context, legacyHostnameResolution bool) (string, error) {
	var instanceID string
	var err error
	if legacyHostnameResolution {
		instanceID, err = ec2GetLegacyResolutionInstanceID(ctx)
	} else {
		instanceID, err = ec2GetInstanceID(ctx)
	}
	if err == nil {
		err = validate.ValidHostname(instanceID)
		if err == nil {
			return instanceID, nil
		}
		return "", fmt.Errorf("EC2 instance ID is not a valid hostname: %s", err)
	}
	return "", fmt.Errorf("Unable to determine hostname from EC2: %s", err)
}

func resolveEC2Hostname(ctx context.Context, currentHostname string, legacyHostnameResolution bool) (string, error) {
	// We use the instance id if we're on an ECS cluster or we're on EC2
	// and the hostname is one of the default ones

	prioritizeEC2Hostname := pkgconfigsetup.Datadog().GetBool("ec2_prioritize_instance_id_as_hostname")

	log.Debugf("Detected a default EC2 hostname: %v", ec2.IsDefaultHostname(currentHostname))
	log.Debugf("ec2_prioritize_instance_id_as_hostname is set to %v", prioritizeEC2Hostname)

	// We use the instance id if we're on an ECS cluster or we're on EC2 and the hostname is one of the default ones
	// or ec2_prioritize_instance_id_as_hostname is set to true
	if env.IsFeaturePresent(env.ECSEC2) || ec2.IsDefaultHostname(currentHostname) || prioritizeEC2Hostname {
		log.Debugf("Trying to fetch hostname from EC2 metadata")
		return getValidEC2Hostname(ctx, legacyHostnameResolution)
	} else if ec2.IsWindowsDefaultHostname(currentHostname) {
		log.Debugf("Default EC2 Windows hostname detected")
		// Display a message when enabling `ec2_use_windows_prefix_detection` would make the hostname resolution change.

		// As we are in the else clause `ec2.IsDefaultHostname(currentHostname)` is false. If
		// `ec2.IsWindowsDefaultHostname(currentHostname)`
		// is `true` that means `ec2_use_windows_prefix_detection` is set to false.
		ec2Hostname, err := getValidEC2Hostname(ctx, legacyHostnameResolution)

		// Check if we get a valid hostname when enabling `ec2_use_windows_prefix_detection` and the hostnames are different.
		if err == nil && ec2Hostname != currentHostname {
			// REMOVEME: This should be removed if/when the default `ec2_use_windows_prefix_detection` is set to true
			log.Infof("The agent resolved your hostname as '%s'. You may want to use the EC2 instance-id ('%s') for the in-app hostname."+
				" For more information: https://docs.datadoghq.com/ec2-use-win-prefix-detection", currentHostname, ec2Hostname)
		}
	}
	return "", fmt.Errorf("not retrieving hostname from AWS: the host is not an ECS instance and other providers already retrieve non-default hostnames")
}

func fromEC2(ctx context.Context, currentHostname string) (string, error) {
	return resolveEC2Hostname(ctx, currentHostname, false)
}

func fromEC2WithLegacyHostnameResolution(ctx context.Context, currentHostname string) (string, error) {
	return resolveEC2Hostname(ctx, currentHostname, true)
}
