// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor/processortest"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
)

type promotionTest struct {
	name                  string
	mode                  ContainerTagPromotionMode
	allowHostnameOverride bool
	taggerTags            []string
	// inResourceAttributes are extra attributes seeded on the incoming resource
	// alongside `container.id: test`. Used to verify preservation semantics
	// (e.g. a pre-existing `datadog.container.tag.<X>` set by the sender).
	inResourceAttributes  map[string]any
	outResourceAttributes map[string]any
}

// promotionTests exercises writeTagAttribute's three-mode logic plus the
// idempotency / convention / USM / host-override exemptions. Cases pivot on
// `mode` and on the kind of tag the tagger emits.
//
// The test vehicle is the traces processor — the promotion logic lives in
// shared ProcessTags() / writeTagAttribute() helpers and behaves identically
// across signals.
var promotionTests = []promotionTest{
	// ---- off (explicit) ----
	{
		name:       "off, custom tag stays unprefixed",
		mode:       ContainerTagPromotionOff,
		taggerTags: []string{"test_tag:bar"},
		outResourceAttributes: map[string]any{
			"container.id": "test",
			"test_tag":     "bar",
		},
	},

	// ---- duplicate ----
	{
		name:       "duplicate, custom tag is duplicated under prefixed key",
		mode:       ContainerTagPromotionDuplicate,
		taggerTags: []string{"test_tag:bar"},
		outResourceAttributes: map[string]any{
			"container.id":                   "test",
			"test_tag":                       "bar",
			"datadog.container.tag.test_tag": "bar",
		},
	},
	{
		name:       "duplicate, DD-convention key (pod_name) is exempt from prefixing",
		mode:       ContainerTagPromotionDuplicate,
		taggerTags: []string{"pod_name:foo"},
		outResourceAttributes: map[string]any{
			"container.id": "test",
			"pod_name":     "foo",
		},
	},
	{
		// `runtime` is a DD-name that lives in attributes.ContainerMappings values
		// but NOT in attributes.KubernetesDDTags. Guards against a regression where
		// only KubernetesDDTags is consulted for exempts.
		name:       "duplicate, DD-name from ContainerMappings values (runtime) is exempt",
		mode:       ContainerTagPromotionDuplicate,
		taggerTags: []string{"runtime:crio"},
		outResourceAttributes: map[string]any{
			"container.id": "test",
			"runtime":      "crio",
		},
	},
	{
		// `kube_service` lives in KubernetesDDTags but NOT in ContainerMappings —
		// trace-agent's container-tag promotion does not recognize it, so IAP
		// MUST prefix it. Regression guard against re-adding KubernetesDDTags
		// to the exempt set (which would strand this key as a span attribute
		// instead of promoting it into `_dd.tags.container`).
		name:       "duplicate, KubernetesDDTags-only key (kube_service) IS promoted",
		mode:       ContainerTagPromotionDuplicate,
		taggerTags: []string{"kube_service:mysvc"},
		outResourceAttributes: map[string]any{
			"container.id":                       "test",
			"kube_service":                       "mysvc",
			"datadog.container.tag.kube_service": "mysvc",
		},
	},
	{
		// Defense-in-depth: the tagger does not emit OTel-format keys in practice.
		// This case documents that if such a key ever slipped through, it would
		// stay under its canonical name rather than gaining a nonsensical
		// `datadog.container.tag.k8s.pod.name` twin.
		name:       "duplicate, OTel-semconv key (k8s.pod.name) is exempt (defense-in-depth)",
		mode:       ContainerTagPromotionDuplicate,
		taggerTags: []string{"k8s.pod.name:foo"},
		outResourceAttributes: map[string]any{
			"container.id": "test",
			"k8s.pod.name": "foo",
		},
	},
	{
		name:       "duplicate, USM tag (service) flows through USM path, not duplicated",
		mode:       ContainerTagPromotionDuplicate,
		taggerTags: []string{"service:svc"},
		outResourceAttributes: map[string]any{
			"container.id": "test",
			"service.name": "svc",
		},
	},

	// ---- rename ----
	{
		name:       "rename, custom tag is written only under prefixed key",
		mode:       ContainerTagPromotionRename,
		taggerTags: []string{"test_tag:bar"},
		outResourceAttributes: map[string]any{
			"container.id":                   "test",
			"datadog.container.tag.test_tag": "bar",
		},
	},
	{
		name:       "rename, DD-convention key (pod_name) is exempt (kept under raw key)",
		mode:       ContainerTagPromotionRename,
		taggerTags: []string{"pod_name:foo"},
		outResourceAttributes: map[string]any{
			"container.id": "test",
			"pod_name":     "foo",
		},
	},
	{
		// Symmetric to the duplicate/runtime case above — `runtime` is a
		// ContainerMappings-values-only DD-name and must not lose its raw key
		// under rename.
		name:       "rename, DD-name from ContainerMappings values (runtime) is exempt",
		mode:       ContainerTagPromotionRename,
		taggerTags: []string{"runtime:crio"},
		outResourceAttributes: map[string]any{
			"container.id": "test",
			"runtime":      "crio",
		},
	},
	{
		// Symmetric to the duplicate/kube_service case — `pod_phase` lives in
		// KubernetesDDTags but NOT in ContainerMappings values, so it MUST be
		// renamed rather than exempt. Regression guard against KubernetesDDTags
		// creeping back into the exempt set.
		name:       "rename, KubernetesDDTags-only key (pod_phase) IS renamed",
		mode:       ContainerTagPromotionRename,
		taggerTags: []string{"pod_phase:running"},
		outResourceAttributes: map[string]any{
			"container.id":                    "test",
			"datadog.container.tag.pod_phase": "running",
		},
	},
	{
		// Defense-in-depth counterpart of the duplicate/k8s.pod.name case.
		name:       "rename, OTel-semconv key (k8s.pod.name) is exempt (defense-in-depth)",
		mode:       ContainerTagPromotionRename,
		taggerTags: []string{"k8s.pod.name:foo"},
		outResourceAttributes: map[string]any{
			"container.id": "test",
			"k8s.pod.name": "foo",
		},
	},
	{
		name:       "rename, USM tag (service) flows through USM path",
		mode:       ContainerTagPromotionRename,
		taggerTags: []string{"service:svc"},
		outResourceAttributes: map[string]any{
			"container.id": "test",
			"service.name": "svc",
		},
	},

	// ---- idempotency ----
	{
		name:       "duplicate, already-prefixed tag is not double-prefixed",
		mode:       ContainerTagPromotionDuplicate,
		taggerTags: []string{"datadog.container.tag.team:platform"},
		outResourceAttributes: map[string]any{
			"container.id":               "test",
			"datadog.container.tag.team": "platform",
		},
	},

	// ---- hostname coexistence ----
	{
		name:                  "rename + allow_hostname_override, datadog.host.name keeps raw key",
		mode:                  ContainerTagPromotionRename,
		allowHostnameOverride: true,
		taggerTags:            []string{"test_tag:bar"},
		outResourceAttributes: map[string]any{
			"container.id":                   "test",
			"datadog.container.tag.test_tag": "bar",
			"datadog.host.name":              "test-host",
		},
	},

	// ---- preserve user-supplied prefixed key ----
	{
		// Regression guard: if the sender already put a datadog.container.tag.X
		// attribute on the resource (typical workaround before this feature
		// existed), the processor must not overwrite it with the tagger's copy.
		// Raw key is still written from the tagger (existing IAP semantic).
		name:       "duplicate, pre-existing user-set prefixed key is preserved",
		mode:       ContainerTagPromotionDuplicate,
		taggerTags: []string{"team:tagger-value"},
		inResourceAttributes: map[string]any{
			"datadog.container.tag.team": "user-canonical-value",
		},
		outResourceAttributes: map[string]any{
			"container.id":               "test",
			"team":                       "tagger-value",
			"datadog.container.tag.team": "user-canonical-value",
		},
	},
	{
		// Symmetric to the duplicate case: rename must also preserve the
		// user-supplied prefixed key. In rename mode the raw key is not
		// written, so the user's value is the only value in the resource.
		name:       "rename, pre-existing user-set prefixed key is preserved",
		mode:       ContainerTagPromotionRename,
		taggerTags: []string{"team:tagger-value"},
		inResourceAttributes: map[string]any{
			"datadog.container.tag.team": "user-canonical-value",
		},
		outResourceAttributes: map[string]any{
			"container.id":               "test",
			"datadog.container.tag.team": "user-canonical-value",
		},
	},
}

// TestKnownConventionKeysCoversContainerMappings is a structural invariant
// test: it asserts that knownConventionKeys — the exempt-set consulted by
// writeTagAttribute — covers exactly the keys that trace-agent's
// ConsumeContainerTagsFromResource recognizes for container-tag promotion:
// sources 1 (OTel semconv keys of ContainerMappings) and 3 (values of
// ContainerMappings, mirroring trace-agent's `containerDDTags`).
//
// This guards against two regressions point-witness tests miss:
//
//  1. Refactor that hard-codes the exempt list and drops a canonical entry
//     (e.g. someone forgets `runtime`).
//  2. Extension of ContainerMappings upstream — new OTel key or new DD-name —
//     that is not reflected in the exempt-set builder.
//
// If this fails, either the builder in common.go is broken, or ContainerMappings
// has grown and needs to be re-audited against IAP's promotion logic.
func TestKnownConventionKeysCoversContainerMappings(t *testing.T) {
	for otelKey, ddName := range attributes.ContainerMappings {
		_, ok := knownConventionKeys[otelKey]
		assert.Truef(t, ok, "ContainerMappings OTel key %q must be exempt from container-tag promotion", otelKey)
		_, ok = knownConventionKeys[ddName]
		assert.Truef(t, ok, "ContainerMappings DD name %q must be exempt from container-tag promotion", ddName)
	}
}

// TestKnownConventionKeysDoesNotOverExempt is the negative-space guard for
// the invariant above. It asserts that the broader `KubernetesDDTags` set
// (used by TagsFromAttributes for OTLP→metric-tag recognition, NOT for
// container-tag promotion) is NOT wholesale copied into the exempt-set.
// Specifically, keys that live in KubernetesDDTags but are absent from
// ContainerMappings values (`kube_service`, `pod_phase`, `kube_qos`,
// `kube_priority_class`, `image_id`, `docker_image`, `git.commit.sha`, ...)
// must NOT be exempt — otherwise the container-tag-promotion feature silently
// strands them as span attributes instead of promoting them.
func TestKnownConventionKeysDoesNotOverExempt(t *testing.T) {
	containerDDNames := make(map[string]struct{}, len(attributes.ContainerMappings))
	for _, ddName := range attributes.ContainerMappings {
		containerDDNames[ddName] = struct{}{}
	}
	var overExempted []string
	for k := range attributes.KubernetesDDTags {
		if _, isContainerTag := containerDDNames[k]; isContainerTag {
			continue // legitimately exempt via ContainerMappings values
		}
		if _, exempted := knownConventionKeys[k]; exempted {
			overExempted = append(overExempted, k)
		}
	}
	assert.Emptyf(t, overExempted,
		"KubernetesDDTags keys that trace-agent's container-tag promotion does NOT recognize must not be exempt; got over-exempted: %v",
		overExempted)
}

func TestContainerTagPromotion(t *testing.T) {
	for _, tt := range promotionTests {
		t.Run(tt.name, func(t *testing.T) {
			next := new(consumertest.TracesSink)
			cfg := &Config{
				Cardinality:                types.LowCardinality,
				AllowHostnameOverride:      tt.allowHostnameOverride,
				TraceContainerTagPromotion: tt.mode,
			}
			tc := testutil.NewTestTaggerClient()
			tc.TagMap["container_id://test"] = tt.taggerTags

			factory := NewFactoryForAgent(tc, func(_ context.Context) (string, error) {
				return "test-host", nil
			})
			tp, err := factory.CreateTraces(
				context.Background(),
				processortest.NewNopSettings(Type),
				cfg,
				next,
			)
			require.NoError(t, err)
			require.NotNil(t, tp)

			ctx := context.Background()
			require.NoError(t, tp.Start(ctx, nil))

			td := ptrace.NewTraces()
			rs := td.ResourceSpans().AppendEmpty()
			in := map[string]any{"container.id": "test"}
			for k, v := range tt.inResourceAttributes {
				in[k] = v
			}
			//nolint:errcheck
			rs.Resource().Attributes().FromRaw(in)
			rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty().SetName("span")

			cErr := tp.ConsumeTraces(ctx, td)
			assert.NoError(t, cErr)
			assert.NoError(t, tp.Shutdown(ctx))

			assert.Len(t, next.AllTraces(), 1)
			out := next.AllTraces()[0].ResourceSpans().At(0)
			assert.EqualValues(t, tt.outResourceAttributes, out.Resource().Attributes().AsRaw())
		})
	}
}
