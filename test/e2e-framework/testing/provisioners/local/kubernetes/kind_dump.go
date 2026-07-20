// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package localkubernetes

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"k8s.io/client-go/tools/clientcmd"

	k8sutils "github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/k8s"
)

// DiagnoseFunc dumps the state of the local Kind cluster. Since a local Docker daemon
// only ever runs one cluster at a time in CI/dev use, it doesn't attempt to match
// stackName to a specific cluster and instead dumps whichever cluster kind reports.
func DiagnoseFunc(ctx context.Context, _ string) (string, error) {
	out, err := exec.CommandContext(ctx, "sh", "-c", `kind get kubeconfig --name "$(kind get clusters | head -n 1)"`).Output()
	if err != nil {
		return "", fmt.Errorf("failed to get kind kubeconfig: %w", err)
	}

	kubeconfig, err := clientcmd.Load(out)
	if err != nil {
		return "", fmt.Errorf("failed to parse kubeconfig: %w", err)
	}

	var dump strings.Builder
	if err := k8sutils.DumpK8sClusterState(ctx, kubeconfig, &dump); err != nil {
		return "", fmt.Errorf("failed to dump cluster state: %w", err)
	}

	return fmt.Sprintf("Dumping Kind cluster state:\n%s", dump.String()), nil
}
