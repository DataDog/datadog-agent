// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package buildprovider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentbuild"
)

// downloadPipelinePackage fetches the exact datadog-agent DEB a GitLab
// pipeline produced, through the repository's own package.download invoke
// task (the same task operators use by hand), then describes and verifies it
// through the existing-package adapter: the digest recorded in the receipt is
// computed over the downloaded file, so the pin travels with the artifact.
// The task needs `dda` on PATH and network access to the testing bucket.
func downloadPipelinePackage(ctx context.Context, pipeline int, r Request) (agentbuild.Result, error) {
	dir := filepath.Join(r.OutputDir, fmt.Sprintf("pipeline-%d", pipeline))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return agentbuild.Result{}, err
	}
	if _, err := agentbuild.Run(ctx, agentbuild.Invocation{
		Program: "dda",
		Args: []string{
			"inv", "--", "package.download",
			fmt.Sprintf("--pipeline=%d", pipeline),
			"--binary=agent", "--type=deb",
			fmt.Sprintf("--arch=%s", r.Target.Arch),
			"--no-extract", fmt.Sprintf("--path=%s", dir),
		},
		StreamOutput: true,
	}); err != nil {
		return agentbuild.Result{}, fmt.Errorf("downloading pipeline %d: %w", pipeline, err)
	}
	debs, err := filepath.Glob(filepath.Join(dir, "*.deb"))
	if err != nil {
		return agentbuild.Result{}, err
	}
	if len(debs) != 1 {
		return agentbuild.Result{}, fmt.Errorf("pipeline %d download did not yield exactly one DEB (found %d)", pipeline, len(debs))
	}
	return r.Adapter.ExistingPackage(ctx, debs[0], "", r.Target)
}
