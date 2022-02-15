// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package docker

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestServiceUp(t *testing.T) {
	expectedTags := []string{}

	sender.AssertServiceCheck(t, "docker.service_up", metrics.ServiceCheckOK, "", expectedTags, "")
}
