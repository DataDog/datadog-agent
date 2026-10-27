// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

// The container tags hash returned in the Datadog-Container-Tags-Hash header
// is folded into the tracers' Database Monitoring (DBM) and Data Streams
// Monitoring (DSM) base hash, so any change to serviceOriginTags affects both
// products.
//
// This file is owned by @DataDog/database-monitoring (see CODEOWNERS), and
// container_tags_hash_test.go by @DataDog/data-streams-monitoring, so changing
// the allowlist requires an approval from both teams. If you need to change
// these tests, please reach out to #database-monitoring first.

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestServiceOriginTagsAllowlistDBM pins the exact set of tags allowed into the
// container tags hash.
func TestServiceOriginTagsAllowlistDBM(t *testing.T) {
	expected := map[string]struct{}{
		"kube_deployment":     {},
		"kube_cronjob":        {},
		"kube_container_name": {},
		"kube_namespace":      {},
		"kube_app_name":       {},
		"kube_app_managed_by": {},
		"service":             {},
		"short_image":         {},
		"kube_cluster_name":   {},
	}
	assert.Equal(t, expected, serviceOriginTags, "serviceOriginTags changed: this affects the DBM base hash, please get a review from @DataDog/database-monitoring")
}
