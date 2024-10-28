// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package docker

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
)

func init() {
	registerComposeFile("exitcode.compose")
}

func TestContainerExit(t *testing.T) {
	expectedTags := []string{
		"docker_image:datadog/docker-library:busybox_1_28_0",
		"image_name:datadog/docker-library",
		"short_image:docker-library",
		"image_tag:busybox_1_28_0",
		"highcardlabeltag:exithigh",
		"lowcardlabeltag:exitlow",
		"highcardenvtag:exithighenv",
		"lowcardenvtag:exitlowenv",
	}
	sender.AssertServiceCheck(t, "docker.exit", servicecheck.ServiceCheckOK, "", append(expectedTags, "container_name:exitcode-exit0-1"), "Container exitcode-exit0-1 exited with 0")
	sender.AssertServiceCheck(t, "docker.exit", servicecheck.ServiceCheckCritical, "", append(expectedTags, "container_name:exitcode-exit1-1"), "Container exitcode-exit1-1 exited with 1")
	sender.AssertServiceCheck(t, "docker.exit", servicecheck.ServiceCheckCritical, "", append(expectedTags, "container_name:exitcode-exit54-1"), "Container exitcode-exit54-1 exited with 54")
}
