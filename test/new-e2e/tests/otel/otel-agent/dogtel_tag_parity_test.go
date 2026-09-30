// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otelagent

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	otelstandalone "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/otel-standalone"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

//go:embed config/dogtel-tag-parity-kubelet.yml
var dogtelTagParityKubeletConfig string

//go:embed config/dogtel-tag-parity-nodefilter.yml
var dogtelTagParityNodefilterConfig string

const (
	// tagParityCollectorAttr is the resource attribute each otel-agent marks
	// the payloads it exports with, set to the workloadmeta collector it runs.
	tagParityCollectorAttr = "e2e.workloadmeta.collector"

	// tagParityNodefilterName and tagParityNodefilterNamespace locate the
	// nodefilter otel-agent's Service, which the kubelet one forwards to.
	tagParityNodefilterName      = "nodefilter-otel-agent"
	tagParityNodefilterNamespace = "datadog-nodefilter"
)

// dogtelTagParityTestSuite checks that a standalone otel-agent tags OTLP
// telemetry the same whether its tagger learns pods from the kubelet or from
// the API server through nodefilter.
//
// It deploys two standalone otel-agents on the same node, each with only the
// RBAC its workloadmeta collector documents. The calendar app sends to the one
// that opts in to the kubelet collector, which both tags and exports the
// telemetry and forwards it, as received, to the one running nodefilter, which
// tags and exports it too. Both export to the same fakeintake, each marking
// its payloads with tagParityCollectorAttr, so that the tests can pair up the
// two copies of each span, log and metric and compare their tags.
type dogtelTagParityTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// kubeletAPIRule grants the kubelet API access the kubelet collector needs to
// list pods: the kubelet authorizes its /pods endpoint as get on nodes/proxy.
func kubeletAPIRule() *rbacv1.PolicyRuleArgs {
	return &rbacv1.PolicyRuleArgs{
		ApiGroups: pulumi.StringArray{pulumi.String("")},
		Resources: pulumi.StringArray{pulumi.String("nodes/proxy")},
		Verbs:     pulumi.StringArray{pulumi.String("get")},
	}
}

// dogtelTagParityProvisioner deploys the nodefilter otel-agent, then the
// kubelet one, which it returns as the environment's agent: the one the
// calendar app sends to.
func dogtelTagParityProvisioner() provisioners.TypedProvisioner[environments.Kubernetes] {
	return standaloneOTelAgentProvisioner(func(e config.Env, kubeProvider *kubernetes.Provider, fi *fakeintakeComp.Fakeintake) (*agent.KubernetesAgent, error) {
		_, err := otelstandalone.K8sAppDefinition(e, kubeProvider, tagParityNodefilterNamespace, dogtelTagParityNodefilterConfig, fi,
			otelstandalone.WithName(tagParityNodefilterName),
			otelstandalone.WithClusterRoleRules(podReadRule()),
		)
		if err != nil {
			return nil, err
		}
		return otelstandalone.K8sAppDefinition(e, kubeProvider, "datadog", dogtelTagParityKubeletConfig, fi,
			otelstandalone.WithClusterRoleRules(kubeletAPIRule()),
		)
	})
}

// TestDogtelTagParity is the entry point for the dogtelTagParityTestSuite. Its
// name is at most 20 characters, the limit TestDogtelStandalone documents.
func TestDogtelTagParity(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &dogtelTagParityTestSuite{},
		e2e.WithProvisioner(dogtelTagParityProvisioner()),
		e2e.WithCoverageRequired(map[string]bool{
			"agent":      false,
			"otel-agent": true,
		}),
	)
}

func (s *dogtelTagParityTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	utils.TestCalendarApp(s, false, utils.CalendarService)
}

// TestWorkloadmetaCollectors checks that each otel-agent collects pods through
// the workloadmeta collector it is meant to compare.
func (s *dogtelTagParityTestSuite) TestWorkloadmetaCollectors() {
	assertWorkloadmetaCollector(s, "datadog", s.Env().Agent.LinuxNodeAgent.LabelSelectors["app"], "kubelet")
	assertWorkloadmetaCollector(s, tagParityNodefilterNamespace, tagParityNodefilterName, "nodefilter")
}

// TestTraceTagParity compares the container tags and span attributes each
// otel-agent exports the calendar app's spans with.
func (s *dogtelTagParityTestSuite) TestTraceTagParity() {
	s.assertTagParity(func(c *assert.CollectT, items taggedItems) {
		traces, err := s.Env().FakeIntake.Client().GetTraces()
		require.NoError(c, err)
		for _, trace := range traces {
			for _, tp := range trace.TracerPayloads {
				var containerTags []string
				if ctags, ok := tp.Tags["_dd.tags.container"]; ok {
					containerTags = strings.Split(ctags, ",")
				}
				for _, chunk := range tp.Chunks {
					for _, span := range chunk.Spans {
						if span.Service != utils.CalendarService {
							continue
						}
						key := strconv.FormatUint(span.SpanID, 10)
						collector := span.Meta[tagParityCollectorAttr]
						items.add(collector, key, "hostname:"+tp.Hostname)
						for _, tag := range containerTags {
							items.add(collector, key, "container_tag."+tag)
						}
						for k, v := range span.Meta {
							if k != tagParityCollectorAttr {
								items.add(collector, key, "meta."+k+":"+v)
							}
						}
					}
				}
			}
		}
		items.assertParity(c, 5, "container_tag.kube_ownerref_kind:replicaset")
	})
}

// TestMetricTagParity compares the tags and resources each otel-agent exports
// the calendar app's counter with, across all of its series.
func (s *dogtelTagParityTestSuite) TestMetricTagParity() {
	s.assertTagParity(func(c *assert.CollectT, items taggedItems) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("calendar-rest-go.api.counter",
			fakeintake.WithTags[*aggregator.MetricSeries]([]string{"service:" + utils.CalendarService}))
		require.NoError(c, err)
		for _, series := range metrics {
			// Series don't carry an identity both otel-agents would share, so
			// compare the union of each one's.
			var collector string
			var tags []string
			for _, tag := range series.Tags {
				if value, ok := strings.CutPrefix(tag, tagParityCollectorAttr+":"); ok {
					collector = value
				} else {
					tags = append(tags, "tag."+tag)
				}
			}
			for _, resource := range series.Resources {
				tags = append(tags, "resource."+resource.Type+":"+resource.Name)
			}
			items.add(collector, "", tags...)
		}
		items.assertParity(c, 1, "tag.kube_ownerref_kind:replicaset")
	})
}

// TestLogTagParity compares the tags and attributes each otel-agent exports
// the calendar app's logs with.
func (s *dogtelTagParityTestSuite) TestLogTagParity() {
	s.assertTagParity(func(c *assert.CollectT, items taggedItems) {
		logs, err := s.Env().FakeIntake.Client().FilterLogs(utils.CalendarService)
		require.NoError(c, err)
		for _, log := range logs {
			attrs := make(map[string]any)
			require.NoError(c, json.Unmarshal([]byte(log.Message), &attrs))
			key := fmt.Sprint(attrs["otel.timestamp"], " ", attrs["message"])
			collector := fmt.Sprint(attrs[tagParityCollectorAttr])
			delete(attrs, tagParityCollectorAttr)
			// ddtags lists the same tags in an order that may differ from one
			// export to the next.
			if ddtags, ok := attrs["ddtags"].(string); ok {
				for _, tag := range strings.Split(ddtags, ",") {
					if !strings.HasPrefix(tag, tagParityCollectorAttr+":") {
						items.add(collector, key, "ddtags."+tag)
					}
				}
				delete(attrs, "ddtags")
			}
			items.add(collector, key, "hostname:"+log.HostName)
			for _, tag := range log.Tags {
				if !strings.HasPrefix(tag, tagParityCollectorAttr+":") {
					items.add(collector, key, "tag."+tag)
				}
			}
			for k, v := range attrs {
				items.add(collector, key, fmt.Sprintf("attr.%s:%v", k, v))
			}
		}
		items.assertParity(c, 5, "tag.kube_ownerref_kind:replicaset")
	})
}

// assertTagParity polls fakeintake, from an empty intake, until collect finds
// the two otel-agents' copies of enough payloads, all tagged the same.
func (s *dogtelTagParityTestSuite) assertTagParity(collect func(*assert.CollectT, taggedItems)) {
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
	s.EventuallyWithT(func(c *assert.CollectT) {
		collect(c, taggedItems{})
	}, 5*time.Minute, 10*time.Second)
}

// taggedItems holds the tags each otel-agent exported items (spans, logs or
// metric series) with, by workloadmeta collector ("kubelet" or "nodefilter"),
// then by a key that identifies the same item in both otel-agents' exports.
type taggedItems map[string]map[string]map[string]struct{}

func (t taggedItems) add(collector, key string, tags ...string) {
	if t[collector] == nil {
		t[collector] = make(map[string]map[string]struct{})
	}
	if t[collector][key] == nil {
		t[collector][key] = make(map[string]struct{})
	}
	for _, tag := range tags {
		t[collector][key][tag] = struct{}{}
	}
}

// assertParity checks that both otel-agents exported the same tags with each
// item they both exported, and that at least minMatched of these items
// carried enrichedTag, i.e. that the comparison covers enriched items. Items
// only one otel-agent exported so far are left out: the other one may just not
// have flushed them yet.
func (t taggedItems) assertParity(c *assert.CollectT, minMatched int, enrichedTag string) {
	kubelet, nodefilter := t["kubelet"], t["nodefilter"]
	matched, enriched := 0, 0
	// Differences are grouped by content, as they usually repeat on every item.
	diffs := make(map[string][]string)
	for key, kubeletTags := range kubelet {
		nodefilterTags, ok := nodefilter[key]
		if !ok {
			continue
		}
		matched++
		if _, ok := kubeletTags[enrichedTag]; ok {
			enriched++
		}
		onlyKubelet, onlyNodefilter := tagsNotIn(kubeletTags, nodefilterTags), tagsNotIn(nodefilterTags, kubeletTags)
		if len(onlyKubelet) > 0 || len(onlyNodefilter) > 0 {
			diff := fmt.Sprintf("only from kubelet: %q, only from nodefilter: %q", onlyKubelet, onlyNodefilter)
			diffs[diff] = append(diffs[diff], key)
		}
	}
	require.GreaterOrEqualf(c, enriched, minMatched,
		"%d items exported by both otel-agents, of which %d tagged %q (kubelet exported %d items, nodefilter %d)",
		matched, enriched, enrichedTag, len(kubelet), len(nodefilter))
	for diff, keys := range diffs {
		assert.Failf(c, "tags differ between the kubelet and nodefilter collectors",
			"%s, on %d of %d items (e.g. %q)", diff, len(keys), matched, keys[0])
	}
}

// tagsNotIn returns the tags of a that aren't in b, sorted.
func tagsNotIn(a, b map[string]struct{}) []string {
	var tags []string
	for tag := range a {
		if _, ok := b[tag]; !ok {
			tags = append(tags, tag)
		}
	}
	slices.Sort(tags)
	return tags
}
