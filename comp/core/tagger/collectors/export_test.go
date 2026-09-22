// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"

// Exposed for the external collectors_test package only (this file is compiled
// in tests alone). That package exists so tests can use the real tag store,
// which the internal test package cannot import: tagstore imports collectors.

// ProcessEventsForTest runs processEvents on the given bundle.
func (c *WorkloadMetaCollector) ProcessEventsForTest(bundle workloadmeta.EventBundle) {
	c.processEvents(bundle)
}

// InitK8sResourcesMetaAsTagsForTest configures resource labels and annotations
// as tags.
func (c *WorkloadMetaCollector) InitK8sResourcesMetaAsTagsForTest(labelsAsTags, annotationsAsTags map[string]map[string]string) {
	c.initK8sResourcesMetaAsTags(labelsAsTags, annotationsAsTags)
}
