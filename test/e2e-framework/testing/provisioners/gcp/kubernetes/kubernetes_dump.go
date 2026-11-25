// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gcpkubernetes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	gcpapi "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"k8s.io/client-go/tools/clientcmd"

	kubecomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/gcp"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	k8sutils "github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/k8s"
	sshutils "github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/ssh"
)

func DumpOpenshiftClusterState(ctx context.Context, name string) (ret string, err error) {
	var out strings.Builder
	defer func() {
		ret = out.String()
	}()

	fmt.Fprintf(&out, "\nstack name: '%s'\n", name)

	// Create GCP client using default provided env var GOOGLE_APPLICATION_CREDENTIALS
	gcpClient, err := gcpapi.NewInstancesRESTClient(ctx)
	if err != nil {
		gcpEnvVarContent := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
		err = fmt.Errorf(
			"failed to init GCP instance client using env var GOOGLE_APPLICATION_CREDENTIALS (content=%s) : %w", gcpEnvVarContent, err)
		return
	}

	// search for the unique instance using labels: manged-by, stack name and unique label on the vm 'kube-provider'
	// to prevent returning the fakeintake vm and only return the vm running openshift.
	instanceFilter := "(labels.managed-by=pulumi) (labels.stack=" + name + ") (labels.kube-provider=openshift)"
	project, zone := gcp.DefaultProjectAndZone(strings.Split(runner.GetProfile().EnvironmentNames(), ","))
	openshiftInstance, err := gcpClient.List(ctx, &computepb.ListInstancesRequest{
		Filter:  &instanceFilter,
		Project: project,
		Zone:    zone,
	}).Next()

	if err != nil {
		err = fmt.Errorf("failed to find the VM running openshift: %w", err)
	}

	if openshiftInstance == nil {
		err = fmt.Errorf("failed to find gcp vm using filter '%s' in project: %s, in zone: %s, no vm found", instanceFilter, project, zone)
		return
	}

	// get the private IP Address
	vmName := openshiftInstance.GetName()
	networks := openshiftInstance.GetNetworkInterfaces()
	if len(networks) == 0 {
		err = fmt.Errorf("the VM %s has 0 interfaces... can't reach for kube config file", vmName)
		return
	}

	if networks[0] == nil {
		err = fmt.Errorf("the VM %s has 1 interface but it's nil :-( can't connect to the VM", vmName)
		return
	}

	vmIP := networks[0].GetNetworkIP()

	fmt.Fprintf(&out, "Found gcp vm running openshift: %s - IP: %s\n", vmName, vmIP)

	// get the kubeconfig file from the VM using ssh
	var errs []error

	sshClient, err := sshutils.SshConnectToInstance(vmIP, "22", "gce")
	if err != nil {
		err = fmt.Errorf("failed to ssh to the VM %s can't extract cluster status: %w", vmName, err)
		errs = append(errs, err)
		return
	}
	defer sshClient.Close()

	sshOutput, err := sshutils.SshRunCommand(
		sshClient,
		"cat ~/.crc/machines/crc/kubeconfig",
		&out,
	)
	if err != nil {
		err = fmt.Errorf("failed to read kubeconfig: %w", err)
		err = errors.Join(append(errs, err)...)
		return
	}

	kubeConfig, err := clientcmd.Load([]byte(kubecomp.OpenShiftKubeconfigForVM(string(sshOutput), vmIP)))
	if err != nil {
		err = fmt.Errorf("failed to load kubeconfig: %w", err)
		err = errors.Join(append(errs, err)...)
		return
	}

	// use the global k8s dump cluster function to dump the cluster state to help debug
	err = k8sutils.DumpK8sClusterState(ctx, kubeConfig, &out)
	if err != nil {
		err = fmt.Errorf("failed to dump cluster state: %w", err)
		errs = append(errs, err)
	}

	return
}
