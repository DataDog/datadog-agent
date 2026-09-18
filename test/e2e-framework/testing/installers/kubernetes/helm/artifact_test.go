// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helm

import (
	"strings"
	"testing"
)

func TestArtifactImageOverridesPreserveHelmSiblingsAndDCA(t *testing.T) {
	values := map[string]interface{}{"agents": map[string]interface{}{"customAgentConfig": map[string]interface{}{"log_level": "debug"}, "image": map[string]interface{}{"pullSecrets": []string{"private-registry"}}}, "clusterAgent": map[string]interface{}{"image": map[string]interface{}{"tag": "7.83.0"}}}
	applyImageArtifact(values, &ImageArtifact{Repository: "localhost/agent", Tag: "7.99.0-e2ectl.123", LocalImageID: "sha256:" + strings.Repeat("a", 64)})
	agents := values["agents"].(map[string]interface{})
	if agents["customAgentConfig"] == nil {
		t.Fatal("custom config clobbered")
	}
	image := agents["image"].(map[string]interface{})
	if image["pullSecrets"] == nil || image["tag"] != "7.99.0-e2ectl.123" {
		t.Fatal(image)
	}
	dca := values["clusterAgent"].(map[string]interface{})["image"].(map[string]interface{})
	if dca["tag"] != "7.83.0" {
		t.Fatal("core image silently retagged Cluster Agent")
	}
}
