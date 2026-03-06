// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package hostnameimpl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/azure"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/gce"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// These variables are overridable for testing.
var (
	isSidecar                        = fargate.IsSidecar
	ec2GetInstanceID                 = ec2.GetInstanceID
	ec2GetLegacyResolutionInstanceID = ec2.GetLegacyResolutionInstanceID
	gceGetHostname                   = gce.GetHostname
	azureGetHostname                 = azure.GetHostname
	osHostname                       = os.Hostname
	fqdnHostname                     = getSystemFQDN
	osHostnameUsable                 = isOSHostnameUsable
)

func fromConfig(ctx context.Context, cfg pkgconfigmodel.Reader, _ string) (string, error) {
	configName := cfg.GetString("hostname")
	if err := validate.ValidHostname(configName); err != nil {
		return "", err
	}
	warnIfNotCanonicalHostname(ctx, cfg, configName)
	return configName, nil
}

func fromHostnameFile(ctx context.Context, cfg pkgconfigmodel.Reader, _ string) (string, error) {
	hostnameFilepath := cfg.GetString("hostname_file")
	if hostnameFilepath == "" {
		return "", errors.New("'hostname_file' configuration is not enabled")
	}

	fileContent, err := os.ReadFile(hostnameFilepath)
	if err != nil {
		return "", fmt.Errorf("could not read hostname from %s: %v", hostnameFilepath, err)
	}

	hostname := strings.TrimSpace(string(fileContent))
	if err := validate.ValidHostname(hostname); err != nil {
		return "", err
	}
	warnIfNotCanonicalHostname(ctx, cfg, hostname)
	return hostname, nil
}

func fromFargate(_ context.Context, _ pkgconfigmodel.Reader, _ string) (string, error) {
	if isSidecar() {
		return "", nil
	}
	return "", errors.New("agent is not running in sidecar mode")
}

func fromGCE(ctx context.Context, _ pkgconfigmodel.Reader, _ string) (string, error) {
	return gceGetHostname(ctx)
}

func fromAzure(ctx context.Context, _ pkgconfigmodel.Reader, _ string) (string, error) {
	return azureGetHostname(ctx)
}

func fromFQDN(ctx context.Context, cfg pkgconfigmodel.Reader, _ string) (string, error) {
	if !osHostnameUsable(ctx, cfg) {
		return "", errors.New("FQDN hostname is not usable")
	}
	if cfg.GetBool("hostname_fqdn") {
		fqdn, err := fqdnHostname()
		if err == nil {
			return fqdn, nil
		}
		return "", fmt.Errorf("unable to get FQDN from system: %s", err)
	}
	return "", errors.New("'hostname_fqdn' configuration is not enabled")
}

func fromOS(ctx context.Context, cfg pkgconfigmodel.Reader, currentHostname string) (string, error) {
	if osHostnameUsable(ctx, cfg) {
		if currentHostname == "" {
			return osHostname()
		}
		return "", errors.New("skipping OS hostname as a previous provider found a valid hostname")
	}
	return "", errors.New("OS hostname is not usable")
}

func getValidEC2Hostname(ctx context.Context, legacyHostnameResolution bool) (string, error) {
	var instanceID string
	var err error
	if legacyHostnameResolution {
		instanceID, err = ec2GetLegacyResolutionInstanceID(ctx)
	} else {
		instanceID, err = ec2GetInstanceID(ctx)
	}
	if err != nil {
		return "", fmt.Errorf("unable to determine hostname from EC2: %s", err)
	}
	if err := validate.ValidHostname(instanceID); err != nil {
		return "", fmt.Errorf("EC2 instance ID is not a valid hostname: %s", err)
	}
	return instanceID, nil
}

func resolveEC2Hostname(ctx context.Context, cfg pkgconfigmodel.Reader, currentHostname string, legacyHostnameResolution bool) (string, error) {
	prioritizeEC2Hostname := cfg.GetBool("ec2_prioritize_instance_id_as_hostname")

	log.Debugf("Detected a default EC2 hostname: %v", ec2.IsDefaultHostname(currentHostname))
	log.Debugf("ec2_prioritize_instance_id_as_hostname is set to %v", prioritizeEC2Hostname)

	ecsManaged := env.IsECSManagedInstancesDaemonMode(cfg)
	if env.IsFeaturePresent(env.ECSEC2) || ecsManaged || ec2.IsDefaultHostname(currentHostname) || prioritizeEC2Hostname {
		log.Debugf("Trying to fetch hostname from EC2 metadata")
		return getValidEC2Hostname(ctx, legacyHostnameResolution)
	} else if ec2.IsWindowsDefaultHostname(currentHostname) {
		log.Debugf("Default EC2 Windows hostname detected")
		ec2Hostname, err := getValidEC2Hostname(ctx, legacyHostnameResolution)
		if err == nil && ec2Hostname != currentHostname {
			// REMOVEME: This should be removed if/when the default `ec2_use_windows_prefix_detection` is set to true
			log.Infof("The agent resolved your hostname as '%s'. You may want to use the EC2 instance-id ('%s') for the in-app hostname."+
				" For more information: https://docs.datadoghq.com/ec2-use-win-prefix-detection", currentHostname, ec2Hostname)
		}
	}
	return "", errors.New("not retrieving hostname from AWS: the host is not an ECS instance and other providers already retrieve non-default hostnames")
}

func fromEC2(ctx context.Context, cfg pkgconfigmodel.Reader, currentHostname string) (string, error) {
	return resolveEC2Hostname(ctx, cfg, currentHostname, false)
}

func fromEC2WithLegacyHostnameResolution(ctx context.Context, cfg pkgconfigmodel.Reader, currentHostname string) (string, error) {
	return resolveEC2Hostname(ctx, cfg, currentHostname, true)
}
