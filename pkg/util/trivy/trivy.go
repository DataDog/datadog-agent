// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trivy

import (
	"context"
	"fmt"
	"os"
	"time"

	cyclonedxgo "github.com/CycloneDX/cyclonedx-go"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/aquasecurity/trivy/pkg/commands/artifact"
	"github.com/aquasecurity/trivy/pkg/fanal/handler"
	"github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/flag"
	"github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
)

type Client struct {
	runner    artifact.Runner
	opts      flag.Options
	marshaler *cyclonedx.Marshaler
}

func NewClient(ctx context.Context) (*Client, error) {
	// TODO: Disable the dpkg handler that was added in our Trivy fork, because
	// it panics when scanning images
	handler.DeregisterPostHandler(types.DpkgPostHandler)

	// Trivy only allows to configure the containerd socket path by setting the
	// "CONTAINERD_ADDRESS" env
	if err := os.Setenv("CONTAINERD_ADDRESS", config.Datadog.GetString("cri_socket_path")); err != nil {
		return nil, err
	}

	fsFlags := &flag.Flags{
		ReportFlagGroup: flag.NewReportFlagGroup(),
		ScanFlagGroup:   flag.NewScanFlagGroup(),
	}

	globalFlags := flag.NewGlobalFlagGroup()

	opts, err := fsFlags.ToOptions("", []string{}, globalFlags, os.Stdout)
	if err != nil {
		return nil, fmt.Errorf("error creating Trivy options: %w", err)
	}

	opts.Format = "cyclonedx"
	opts.Timeout = 5 * time.Minute
	opts.ListAllPkgs = true

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	runner, err := artifact.NewRunner(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("error creating Trivy runner: %w", err)
	}

	marshaler := cyclonedx.NewMarshaler(opts.AppVersion)

	client := Client{
		runner:    runner,
		opts:      opts,
		marshaler: marshaler,
	}

	return &client, nil
}

func (c *Client) ScanImage(ctx context.Context, namespace string, imageName string) (*cyclonedxgo.BOM, error) {
	// Trivy only allows to configure the containerd namespace by setting the
	// "CONTAINERD_NAMESPACE" env.
	// Notice that this means that there cannot be multiple concurrent calls to
	// this function.
	if err := os.Setenv("CONTAINERD_NAMESPACE", namespace); err != nil {
		return nil, err
	}

	c.opts.Target = imageName // TODO: fix

	ctx, cancel := context.WithTimeout(ctx, c.opts.Timeout)
	defer cancel()

	report, err := c.runner.ScanImage(ctx, c.opts)
	if err != nil {
		return nil, fmt.Errorf("error scanning image with Trivy: %w", err)
	}

	sbom, err := c.marshaler.Marshal(report)
	if err != nil {
		return nil, fmt.Errorf("error marshaling Trivy report: %w", err)
	}

	return sbom, nil
}
