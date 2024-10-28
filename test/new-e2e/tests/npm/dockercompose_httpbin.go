// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	_ "embed"

	"github.com/DataDog/test-infra-definitions/components/docker"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed "config/dockercompose_httpbin.yaml"
var dockerHTTPBinComposeYaml string

func dockerHTTPBinCompose() docker.ComposeInlineManifest {
	return docker.ComposeInlineManifest{
		Name:    "httpbin",
		Content: pulumi.String(dockerHTTPBinComposeYaml),
	}
}
