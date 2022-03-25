// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostname

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/azure"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/gce"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/ecs"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func fromConfig(ctx Context, _ string) (string, error) {
	configName := config.Datadog.GetString("hostname")
	err := validate.ValidHostname(configName)
	if err == nil {
		warnCanonicalHostname(ctx, configName)
	}
	return configName, err
}

func fromHostnameFile(ctx context.Context, _ string) (string, error) {
	// Try `hostname_file` config option next
	hostnameFilepath := config.Datadog.GetString("hostname_file")
	if hostnameFilepath == "" {
		return "", fmt.Errorf("no 'hostname_file' configured")
	}

	fileContent, err := ioutil.ReadFile(hostnameFilepath)
	if err != nil {
		return "", fmt.Errorf("Could not read hostname from %s: %v", filename, err)
	}

	hostname := strings.TrimSpace(string(fileContent))

	err = validate.ValidHostname(hostname)
	if err == nil {
		warnCanonicalHostname(ctx, configName)
	}
	return hostname, err
}

func fromFarget(ctx context.Context, _ string) (string, error) {
	// If we're running on fargate we strip the hostname
	if fargate.IsFargateInstance(ctx) {
		return "", nil
	}
	return "", fmt.Errorf("agent is not runnning on Fargate")
}

func fromGCE(ctx context.Context, _ string) (string, error) {
	return gce.GetHostname(ctx)
}

func fromFQDN(ctx context.Context, _ string) (string, error) {
	if !isOSHostnameUsable(ctx) {
		return "", fmt.Errorf("FQDN hostname is not usable")
	}

	fqdn, err := getSystemFQDN() // TODO solve import
	if config.Datadog.GetBool("hostname_fqdn") && err == nil {
		return fqdn, nil
	}
	return "", fmt.Errorf("Unable to get FQDN from system: ", err)
}

func fromOS(ctx context.Context, currentHostname string) (string, error) {
	if isOSHostnameUsable(ctx) && currentHostname == "" {
		return os.Hostname()
	}
	return "", fmt.Errorf("OS hostname is not usable")
}

func getValidEC2Hostname(ctx context.Context) (string, error) {
	instanceID, err := ec2.GetInstanceID(ctx)
	if err == nil {
		err = validate.ValidHostname(instanceID)
		if err == nil {
			return instanceID, nil
		}
		return "", fmt.Errorf("EC2 instance ID is not a valid hostname: %s", err)
	}
	return "", fmt.Errorf("Unable to determine hostname from EC2: %s", err)
}

func fromEC2(ctx context.Context, currentHostname string) (string, error) {
	// We use the instance id if we're on an ECS cluster or we're on EC2
	// and the hostname is one of the default ones

	if ecs.IsECSInstance() || ec2.IsDefaultHostname(currentHostname) {
		return getValidEC2Hostname(ctx)
	} else if ec2.IsWindowsDefaultHostname(currentHostname) {
		// Display a message when enabling `ec2_use_windows_prefix_detection` would make the hostname resolution change.

		// As we are in the else clause `ec2.IsDefaultHostname(currentHostname)` is false. If
		// `ec2.IsWindowsDefaultHostname(currentHostname)`
		// is `true` that means `ec2_use_windows_prefix_detection` is set to false.
		ec2Hostname, err := getValidEC2Hostname(ctx)

		// Check if we get a valid hostname when enabling `ec2_use_windows_prefix_detection` and the hostnames are different.
		if err == nil && ec2Hostname != currentHostname {
			// REMOVEME: This should be removed if/when the default `ec2_use_windows_prefix_detection` is set to true
			log.Infof("The agent resolved your hostname as '%s'. You may want to use the EC2 instance-id ('%s') for the in-app hostname."+
				" For more information: https://docs.datadoghq.com/ec2-use-win-prefix-detection", currentHostname, ec2Hostname)
		}
	}
	return "", fmt.Errorf("not retrieving hostname from AWS: the host is not an ECS instance and other providers already retrieve non-default hostnames")
}

func fromAzure(ctx context.Context, currentHostname string) (string, error) {
	return azure.GetHostname(ctx, nil)
}
