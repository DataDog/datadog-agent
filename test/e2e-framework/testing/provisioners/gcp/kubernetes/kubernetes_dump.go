// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gcpkubernetes

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"

	gcpapi "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"google.golang.org/api/option"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/infra"
)

// func getClusterOutputWithSsh(out *strings.Builder, sshClient *ssh.Client) (err error) {
// 	var errs []error

// 	defer func() {
// 		err = errors.Join(errs...)
// 	}()

// 	crcStatus := "oc status"
// 	crcStatusOutput, err := infra.SshRunCommand(sshClient, crcStatus, out)
// 	if err != nil {
// 		err = fmt.Errorf("failed to run command '%s': %w", crcStatus, err)
// 		errs = append(errs, err)
// 	}
// 	fmt.Fprintf(out, "---------- CRC Status ----------\n%s\n", crcStatusOutput)

// 	prefix := "eval $(crc oc-env); "
// 	getAllCommand := prefix + "oc get all,nodes -o wide -A"
// 	allOutput, err := infra.SshRunCommand(sshClient, getAllCommand, out)
// 	if err != nil {
// 		err = fmt.Errorf("failed to run command '%s': %w", getAllCommand, err)
// 		errs = append(errs, err)
// 	}

// 	fmt.Fprintf(out, "---------- All resources ----------\n%s\n", string(allOutput))

// 	getAllNonRuningPods := prefix + `oc get pod --field-selector='status.phase!=Running' -A -o jsonpath='{range .items[*]}{.metadata.namespace}{" "}{.metadata.name}{","}'`
// 	nonRuningPodsList, err := infra.SshRunCommand(sshClient, getAllNonRuningPods, out)
// 	if err != nil {
// 		err = fmt.Errorf("failed to run command: '%s': %w", getAllNonRuningPods, err)
// 		errs = append(errs, err)
// 		return
// 	}

// 	fmt.Fprintf(out, "---------- All non running pods ----------\n%s\n", strings.ReplaceAll(string(nonRuningPodsList), ",", "\n"))

// 	podDescribeTemplate := "%s oc describe pod --namespace %s %s"
// 	podLogsTemplate := "%s oc logs --all-containers --prefix --namespace %s %s"
// 	previousPodLogsTemplate := "%s oc logs --all-containers=true --prefix=true --previous=true --namespace %s %s"
// 	for _, line := range strings.Split(strings.TrimSpace(string(nonRuningPodsList)), ",") {
// 		parts := strings.Split(strings.TrimSpace(line), " ")
// 		if len(parts) != 2 {
// 			err = fmt.Errorf("failed to find '<namespace> <pod name>' in: %s", line)
// 			continue
// 		}

// 		namespace := parts[0]
// 		name := parts[1]

// 		command := fmt.Sprintf(podDescribeTemplate, prefix, namespace, name)
// 		podDescribe, err := infra.SshRunCommand(sshClient, command, out)
// 		if err != nil {
// 			err = fmt.Errorf("failed to run command: %s: %w", command, err)
// 			errs = append(errs, err)
// 			continue
// 		}

// 		fmt.Fprintf(out, "---------- Failed pod (%s/%s) ----------\n%s\n", namespace, name, string(podDescribe))

// 		command = fmt.Sprintf(podLogsTemplate, prefix, namespace, name)
// 		podLogs, err := infra.SshRunCommand(sshClient, command, out)
// 		if err != nil {
// 			err = fmt.Errorf("failed to read logs for pod %s/%s: %w", err)
// 			errs = append(errs, err)
// 			continue
// 		}

// 		fmt.Fprintf(out, "---------- logs (%s/%s) ----------\n%s\n", namespace, name, podLogs)

// 		command = fmt.Sprintf(previousPodLogsTemplate, prefix, namespace, name)
// 		podLogs, err = infra.SshRunCommand(sshClient, command, out)
// 		if err != nil {
// 			err = fmt.Errorf("failed to read previous logs for pod %s/%s: %w", err)
// 			errs = append(errs, err)
// 			continue
// 		}

// 		fmt.Fprintf(out, "---------- previous logs (%s/%s) ----------\n%s\n", namespace, name, podLogs)

// 	}

// 	return
// }

func DumpOpenshiftClusterState(ctx context.Context, name string) (ret string, err error) {
	var out strings.Builder
	defer func() {
		ret = out.String()
	}()

	fmt.Fprintf(&out, "\nstack name: '%s'\n", name)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		err = fmt.Errorf("failed to read homedir path from env to open gcloud creds: %w", err)
		return
	}

	credentialsPath := path.Join(homeDir, "/gcp-credentials.json")
	gcpClient, err := gcpapi.NewInstancesRESTClient(
		ctx,
		option.WithAuthCredentialsFile(option.ServiceAccount, credentialsPath))
	if err != nil {
		err = fmt.Errorf("failed to init GCP instance client using credentials at '%s' : %w", credentialsPath, err)
		return
	}

	instanceFilter := "(labels.managed-by=pulumi) (labels.stack=" + name + ")"
	project := "datadog-agent-qa"
	zone := "us-central1-a"
	instanceIterator := gcpClient.List(ctx, &computepb.ListInstancesRequest{
		Filter:  &instanceFilter,
		Project: project,
		Zone:    zone,
	}).All()

	var openshiftInstance *computepb.Instance
	for instance := range instanceIterator {
		fmt.Fprintf(&out, "\nInstance: %s\n", instance.GetName())
		if strings.Contains(instance.GetName(), "openshiftvm") {
			openshiftInstance = instance
			break
		}
	}

	if openshiftInstance == nil {
		err = fmt.Errorf("failed to find gcp vm using filter '%s' in project: %s, in zone: %s, no vm found", instanceFilter, project, zone)
		return
	}

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

	var errs []error

	sshClient, err := infra.SshConnectToInstance(vmIP, "22", "gce")
	if err != nil {
		err = fmt.Errorf("failed to ssh to the VM %s can't extract cluster status: %w", vmName, err)
		errs = append(errs, err)
		return
	}
	defer sshClient.Close()

	// err = getClusterOutputWithSsh(&out, sshClient)
	// if err != nil {
	// 	errs = append(errs, err)
	// }

	sshOutput, err := infra.SshRunCommand(
		sshClient,
		"cat .kube/config",
		&out,
	)
	if err != nil {
		err = fmt.Errorf("failed to read kubeconfig under '.kube/config': %w", err)
		err = errors.Join(append(errs, err)...)
		return
	}

	fmt.Fprintf(&out, "Original kubeconfig file:\n%s\n", string(sshOutput))

	kubeConfig, err := clientcmd.Load(sshOutput)
	if err != nil {
		err = fmt.Errorf("failed to load kubeconfig: %w", err)
		err = errors.Join(append(errs, err)...)
	}

	for _, cluster := range kubeConfig.Clusters {
		serverUrl, err := url.Parse(cluster.Server)
		if err != nil {
			fmt.Fprintf(&out, "[SKIP] failed to parse cluster url: %s - %v\n", cluster.Server, err)
			continue
		}
		// use the vm IP
		// on the localhost, kube api listens on 6443
		// on the private IP kube api listens on 8443
		serverUrl.Host = vmIP + ":8443"

		cluster.Server = serverUrl.String()
		cluster.CertificateAuthorityData = nil
		cluster.InsecureSkipTLSVerify = true
	}

	kubeconfigStr, err := clientcmd.Write(*kubeConfig)
	if err != nil {
		err = fmt.Errorf("failed to write kubeconfig to yaml format: %w", err)
		errs = append(errs, err)
	}

	fmt.Fprintf(&out, "---------- Kubeconfig ----------\n%s\n", string(kubeconfigStr))

	err = infra.DumpK8sClusterState(ctx, kubeConfig, &out)
	if err != nil {
		err = fmt.Errorf("failed to dump cluster state: %w", err)
		errs = append(errs, err)
	}

	return
}
