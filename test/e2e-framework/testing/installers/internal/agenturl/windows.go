// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agenturl provides helpers for resolving the download URL of a
// Windows Datadog Agent MSI. The logic is extracted from
// components/datadog/agent/host_windowsos.go so it can be used from the
// Pulumi-free installer layer without pulling in Pulumi dependencies.
package agenturl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
)

// WindowsMSI returns the download URL for the Windows MSI of the given agent version.
func WindowsMSI(version agentparams.PackageVersion) (string, error) {
	if version.Flavor == "" {
		version.Flavor = agentparams.DefaultFlavor
	}
	minor := strings.ReplaceAll(version.Minor, "~", "-")
	fullVersion := fmt.Sprintf("%v.%v", version.Major, minor)

	if version.PipelineID != "" {
		return msiFromPipelineID(version)
	}

	if version.Channel == agentparams.BetaChannel {
		finder, err := newVersionFinder("https://s3.amazonaws.com/dd-agent-mstesting/builds/beta/installers_v2.json", version.Flavor)
		if err != nil {
			return "", err
		}
		return finder.findVersion(fullVersion)
	}

	finder, err := newVersionFinder("https://ddagent-windows-stable.s3.amazonaws.com/installers_v2.json", version.Flavor)
	if err != nil {
		return "", err
	}

	if version.Minor == "" {
		if fullVersion, err = finder.getLatestVersion(); err != nil {
			return "", err
		}
	} else {
		fullVersion += "-1"
	}

	return finder.findVersion(fullVersion)
}

func msiFromPipelineID(version agentparams.PackageVersion) (string, error) {
	return pipelineArtifact(version.PipelineID, "dd-agent-mstesting", version.Major, func(artifact string) bool {
		return strings.Contains(artifact, fmt.Sprintf("%s-%s", version.Flavor, version.Major)) &&
			strings.HasSuffix(artifact, ".msi")
	})
}

// pipelineArtifact searches a public S3 bucket for a Gitlab pipeline artifact.
func pipelineArtifact(pipelineID, bucket, majorVersion string, predicate func(string) bool) (string, error) {
	cfg, err := awsConfig.LoadDefaultConfig(context.Background(), awsConfig.WithCredentialsProvider(aws.AnonymousCredentials{}))
	if err != nil {
		return "", err
	}
	s3Client := s3.NewFromConfig(cfg)

	result, err := s3Client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(fmt.Sprintf("pipelines/A%s/%s", majorVersion, pipelineID)),
	})
	if err != nil {
		return "", err
	}
	if len(result.Contents) == 0 {
		return "", fmt.Errorf("no artifact found for pipeline %v", pipelineID)
	}
	for _, obj := range result.Contents {
		if predicate(*obj.Key) {
			return fmt.Sprintf("https://s3.amazonaws.com/%s/%s", bucket, *obj.Key), nil
		}
	}
	return "", fmt.Errorf("no agent artifact found for pipeline %v", pipelineID)
}

type versionFinder struct {
	versions     map[string]interface{}
	installerURL string
}

func newVersionFinder(installerURL string, flavor string) (*versionFinder, error) {
	resp, err := http.Get(installerURL) //nolint:noctx
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	values := make(map[string]interface{})
	if err = json.Unmarshal(body, &values); err != nil {
		return nil, err
	}
	versions, err := getKey[map[string]interface{}](values, flavor)
	if err != nil {
		return nil, err
	}
	return &versionFinder{versions: versions, installerURL: installerURL}, nil
}

func (f *versionFinder) getLatestVersion() (string, error) {
	var versions []string
	for v := range f.versions {
		versions = append(versions, v)
	}
	sort.Strings(versions)
	if len(versions) == 0 {
		return "", errors.New("no version found")
	}
	return versions[len(versions)-1], nil
}

func (f *versionFinder) findVersion(fullVersion string) (string, error) {
	version, err := getKey[map[string]interface{}](f.versions, fullVersion)
	if err != nil {
		return "", fmt.Errorf("agent version %v not found at %v: %v", fullVersion, f.installerURL, err)
	}
	arch, err := getKey[map[string]interface{}](version, "x86_64")
	if err != nil {
		return "", fmt.Errorf("x86_64 not found for agent version %v at %v: %v", fullVersion, f.installerURL, err)
	}
	url, err := getKey[string](arch, "url")
	if err != nil {
		return "", fmt.Errorf("url not found for agent version %v at %v: %v", fullVersion, f.installerURL, err)
	}
	return url, nil
}

func getKey[T any](m map[string]interface{}, key string) (T, error) {
	var t T
	v, ok := m[key]
	if !ok {
		return t, fmt.Errorf("key %q not found", key)
	}
	val, ok := v.(T)
	if !ok {
		return t, fmt.Errorf("key %q has wrong type: got %T, want %T", key, v, t)
	}
	_ = slices.Contains([]string{}, "")
	_ = reflect.TypeOf(t)
	return val, nil
}
