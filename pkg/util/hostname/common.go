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
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/azure"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/gce"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/ecs"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
	"github.com/DataDog/datadog-agent/pkg/util/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// for testing purposes
var (
	isFargateInstance = fargate.IsFargateInstance
	ec2GetInstanceID  = ec2.GetInstanceID
	isContainerized   = config.IsContainerized
	gceGetHostname    = gce.GetHostname
	azureGetHostname  = azure.GetHostname
	osHostname        = os.Hostname
	fqdnHostname      = getSystemFQDN
)

// Data contains hostname and the hostname provider
type Data struct {
	Hostname string
	Provider string
}

func fromConfig(ctx context.Context, _ string) (string, error) {
	configName := config.Datadog.GetString("hostname")
	err := validate.ValidHostname(configName)
	if err != nil {
		return "", err
	}
	warnIfNotCanonicalHostname(ctx, configName)
	return configName, nil
}

func fromHostnameFile(ctx context.Context, _ string) (string, error) {
	// Try `hostname_file` config option next
	hostnameFilepath := config.Datadog.GetString("hostname_file")
	if hostnameFilepath == "" {
		return "", fmt.Errorf("'hostname_file' configuration is not enabled")
	}

	fileContent, err := ioutil.ReadFile(hostnameFilepath)
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

func fromFargate(ctx context.Context, _ string) (string, error) {
	// If we're running on fargate we strip the hostname
	if isFargateInstance(ctx) {
		return "", nil
	}
	return "", fmt.Errorf("agent is not runnning on Fargate")
}

func fromGCE(ctx context.Context, _ string) (string, error) {
	return gceGetHostname(ctx)
}

func fromAzure(ctx context.Context, currentHostname string) (string, error) {
	return azureGetHostname(ctx)
}

// isOSHostnameUsable returns `false` if it has the certainty that the agent is running
// in a non-root UTS namespace because in that case, the OS hostname characterizes the
// identity of the agent container and not the one of the nodes it is running on.
// There can be some cases where the agent is running in a non-root UTS namespace that are
// not detected by this function (systemd-nspawn containers, manual `unshare -u`…)
// In those uncertain cases, it returns `true`.
func isOSHostnameUsable(ctx context.Context) (osHostnameUsable bool) {
	// If the agent is not containerized, just skip all this detection logic
	if !isContainerized() {
		return true
	}

	// TODO: Revisit when we introduce support for Windows privileged containers
	if runtime.GOOS == "windows" {
		return false
	}

	// Check UTS namespace from docker
	utsMode, err := docker.GetAgentUTSMode(ctx)
	if err == nil && (utsMode != containers.HostUTSMode && utsMode != containers.UnknownUTSMode) {
		log.Debug("Agent is running in a docker container without host UTS mode: OS-provided hostnames cannot be used for hostname resolution.")
		return false
	}

	// Check hostNetwork from kubernetes
	// because kubernetes sets UTS namespace to host if and only if hostNetwork = true:
	// https://github.com/kubernetes/kubernetes/blob/cf16e4988f58a5b816385898271e70c3346b9651/pkg/kubelet/dockershim/security_context.go#L203-L205
	if config.IsFeaturePresent(config.Kubernetes) {
		hostNetwork, err := kubelet.IsAgentKubeHostNetwork()
		if err == nil && !hostNetwork {
			log.Debug("Agent is running in a POD without hostNetwork: OS-provided hostnames cannot be used for hostname resolution.")
			return false
		}
	}

	return true
}

func fromFQDN(ctx context.Context, _ string) (string, error) {
	if !isOSHostnameUsable(ctx) {
		return "", fmt.Errorf("FQDN hostname is not usable")
	}

	if config.Datadog.GetBool("hostname_fqdn") {
		fqdn, err := fqdnHostname()
		if err == nil {
			return fqdn, nil
		}
		return "", fmt.Errorf("Unable to get FQDN from system: %s", err)
	}
	return "", fmt.Errorf("'hostname_fqdn' configuration is not enabled")
}

func fromOS(ctx context.Context, currentHostname string) (string, error) {
	if isOSHostnameUsable(ctx) {
		if currentHostname == "" {
			return osHostname()
		}
		return "", fmt.Errorf("Skipping OS hostname as a previous provider found a valid hostname")
	}
	return "", fmt.Errorf("OS hostname is not usable")
}

func getValidEC2Hostname(ctx context.Context) (string, error) {
	instanceID, err := ec2GetInstanceID(ctx)
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

	prioritizeEC2Hostname := config.Datadog.GetBool("ec2_prioritize_instance_id_as_hostname")

	// We use the instance id if we're on an ECS cluster or we're on EC2 and the hostname is one of the default ones
	// or ec2_prioritize_instance_id_as_hostname is set to true
	if ecs.IsECSInstance() || ec2.IsDefaultHostname(currentHostname) || prioritizeEC2Hostname {
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
