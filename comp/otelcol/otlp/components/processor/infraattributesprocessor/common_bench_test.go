// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/processor/processortest"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// benchTagger returns a tagger holding pods worth of entities, each with
// tagsPerEntity tags, shaped like what workloadmeta produces in a Kubernetes
// deployment.
func benchTagger(pods, tagsPerEntity int) *testutil.TestTaggerClient {
	tc := testutil.NewTestTaggerClient()
	tags := func(prefix string) []string {
		entityTags := make([]string, 0, tagsPerEntity)
		for i := 0; i < tagsPerEntity; i++ {
			entityTags = append(entityTags, prefix+"_tag_"+strconv.Itoa(i)+":value_"+strconv.Itoa(i))
		}
		return entityTags
	}
	for pod := 0; pod < pods; pod++ {
		suffix := strconv.Itoa(pod)
		tc.TagMap[types.NewEntityID(types.ContainerID, "cid-"+suffix).String()] = tags("container")
		tc.TagMap[types.NewEntityID(types.KubernetesPodUID, "pod-"+suffix).String()] = tags("pod")
	}
	tc.TagMap[types.NewEntityID(types.KubernetesDeployment, "ns/dep").String()] = tags("deployment")
	tc.TagMap[types.NewEntityID(types.KubernetesMetadata, "/namespaces//ns").String()] = tags("namespace")
	tc.TagMap[types.NewEntityID(types.KubernetesNode, "node-1").String()] = tags("node")
	tc.TagMap[types.NewEntityID("internal", "global-entity-id").String()] = tags("global")
	return tc
}

// benchMetrics builds a batch of resources spread over pods distinct pods.
// Each resource resolves to five entities, three of which (deployment,
// namespace, node) are shared by the whole batch.
func benchMetrics(resources, pods int) pmetric.Metrics {
	md := pmetric.NewMetrics()
	for i := 0; i < resources; i++ {
		suffix := strconv.Itoa(i % pods)
		rm := md.ResourceMetrics().AppendEmpty()
		attrs := rm.Resource().Attributes()
		attrs.PutStr("container.id", "cid-"+suffix)
		attrs.PutStr("k8s.pod.uid", "pod-"+suffix)
		attrs.PutStr("k8s.namespace.name", "ns")
		attrs.PutStr("k8s.deployment.name", "dep")
		attrs.PutStr("k8s.node.name", "node-1")
		rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty().SetName("bench_metric")
	}
	return md
}

// BenchmarkProcessMetrics measures the per-batch tag enrichment.
//
// Two shapes are covered. "shared" is the common one: a batch collects the
// telemetry of a handful of pods, so most resources resolve to entities an
// earlier resource in the batch already resolved. "distinct" is the worst
// case for memoization -- every resource belongs to its own pod, so only the
// three cluster-wide entities (deployment, namespace, node) ever repeat.
//
// Enrichment is idempotent -- every attribute it writes is already present
// from the second iteration onwards -- so the tagger work being measured is
// the same in every iteration.
func BenchmarkProcessMetrics(b *testing.B) {
	const tagsPerEntity = 10
	for _, resources := range []int{10, 100, 1000} {
		for _, shape := range []string{"shared", "distinct"} {
			pods := 4
			if shape == "distinct" {
				pods = resources
			}
			b.Run(fmt.Sprintf("%s/resources=%d", shape, resources), func(b *testing.B) {
				iamp, err := newInfraAttributesMetricProcessor(
					processortest.NewNopSettings(Type),
					newInfraTagsProcessor(benchTagger(pods, tagsPerEntity), option.None[SourceProviderFunc]()),
					&Config{Cardinality: types.LowCardinality},
				)
				require.NoError(b, err)

				md := benchMetrics(resources, pods)
				ctx := context.Background()
				// Warm the resource attribute maps so that the first timed
				// iteration is not the one that grows them.
				_, err = iamp.processMetrics(ctx, md)
				require.NoError(b, err)

				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := iamp.processMetrics(ctx, md); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
