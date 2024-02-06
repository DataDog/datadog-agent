// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package scanners

import (
	"context"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/awsutils"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/types"

	cdx "github.com/CycloneDX/cyclonedx-go"

	"github.com/aquasecurity/trivy/pkg/fanal/analyzer"
	"github.com/aquasecurity/trivy/pkg/fanal/artifact"
	"github.com/aquasecurity/trivy/pkg/fanal/artifact/vm"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/fanal/walker"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ebs"
)

type ebsClientWithWalk struct {
	*ebs.Client
}

func (e ebsClientWithWalk) WalkSnapshotBlocks(ctx context.Context, input *ebs.ListSnapshotBlocksInput, table map[int32]string) (*ebs.ListSnapshotBlocksOutput, map[int32]string, error) {
	for {
		output, err := e.ListSnapshotBlocks(ctx, input)
		if err != nil {
			return nil, nil, err
		}
		for _, block := range output.Blocks {
			table[*block.BlockIndex] = *block.BlockToken
		}
		output.VolumeSize = aws.Int64(*output.VolumeSize << 30)
		if output.NextToken == nil {
			return output, table, nil
		}
		input.NextToken = output.NextToken
	}
}

// LaunchTrivyHostVM launches a trivy scan on a EBS volume.
func LaunchTrivyHostVM(ctx context.Context, opts types.ScannerOptions) (*cdx.BOM, error) {
	ebsclient := ebs.NewFromConfig(awsutils.GetConfigFromCloudID(ctx, opts.Scan, *opts.SnapshotID))
	trivyCache := newMemoryCache()
	onlyDirs := []string{
		"/etc/*",
		"/usr/lib/*",
		"/var/lib/dpkg/**",
		"/var/lib/rpm/**",
		"/usr/lib/sysimage/**",
		"/lib/apk/**",
	}
	w := walker.NewVM(nil, nil, onlyDirs)
	snapshotID := *opts.SnapshotID
	target := "ebs:" + snapshotID.ResourceName()
	trivyArtifact, err := vm.NewArtifact(target, trivyCache, w, artifact.Option{
		Offline:           true,
		NoProgress:        true,
		DisabledAnalyzers: getTrivyDisabledAnalyzers(analyzer.TypeOSes),
		Parallel:          1,
		SBOMSources:       []string{},
		DisabledHandlers:  []ftypes.HandlerType{ftypes.UnpackagedPostHandler},
		AWSRegion:         opts.SnapshotID.Region(),
	})
	if err != nil {
		return nil, err
	}
	trivyArtifactEBS := trivyArtifact.(*vm.EBS)
	trivyArtifactEBS.SetEBS(ebsClientWithWalk{ebsclient})
	return doTrivyScan(ctx, opts.Scan, trivyArtifact, trivyCache)
}
