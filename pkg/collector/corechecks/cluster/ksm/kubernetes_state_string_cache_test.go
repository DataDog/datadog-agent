// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
)

func TestStringCache(t *testing.T) {
	var c stringCache[string]

	_, found := c.get("a")
	assert.False(t, found)
	c.put("a", "tag-a")
	c.put("b", "tag-b")
	c.endRun()

	// The strings of the previous run are kept
	s, found := c.get("a")
	assert.True(t, found)
	assert.Equal(t, "tag-a", s)
	// Those this run doesn't use are dropped at its end
	c.endRun()

	_, found = c.get("b")
	assert.False(t, found)
	s, found = c.get("a")
	assert.True(t, found)
	assert.Equal(t, "tag-a", s)
}

func TestStringCacheKeepsUnusedStringsForKeepRuns(t *testing.T) {
	c := newStringCache[string](2)
	c.put("a", "tag-a")
	c.endRun()

	// A run that doesn't use the string keeps it, as the aggregator keeps a
	// context that missed a single commit
	c.endRun()
	s, found := c.get("a")
	assert.True(t, found)
	assert.Equal(t, "tag-a", s)
	c.endRun()

	// Two runs that don't use it drop it
	c.endRun()
	c.endRun()
	_, found = c.get("a")
	assert.False(t, found)
}

func TestStringCacheHitKeepsCachedKey(t *testing.T) {
	var c stringCache[string]
	cached := strings.Clone("node-1")
	c.put(cached, "node-1-cluster")
	c.endRun()

	// A hit with an equal key from another object doesn't make the cache keep it
	_, found := c.get(strings.Clone("node-1"))
	require.True(t, found)
	for key := range c.entries {
		assert.True(t, sameString(cached, key), "the cache keeps the key of the caller")
	}
}

func TestStringCacheShrinksAfterPeak(t *testing.T) {
	var c stringCache[int]
	for i := range 100 {
		c.put(i, strconv.Itoa(i))
	}
	c.endRun()
	assert.Equal(t, 100, c.peak)

	// The next run only uses a few strings: the map is rebuilt for them
	for i := range 10 {
		_, found := c.get(i)
		assert.True(t, found)
	}
	c.endRun()
	assert.Len(t, c.entries, 10)
	assert.Equal(t, 10, c.peak)
	for i := range 10 {
		s, found := c.get(i)
		assert.True(t, found)
		assert.Equal(t, strconv.Itoa(i), s)
	}
}

func TestStringCacheShrinksAfterTurnover(t *testing.T) {
	var c stringCache[int]
	for i := range 100 {
		c.put(i, strconv.Itoa(i))
	}
	c.endRun()

	// A run replaces all the strings, like a rollout replaces all the pods
	for i := 100; i < 200; i++ {
		c.put(i, strconv.Itoa(i))
	}
	assert.Equal(t, 200, c.peak)
	c.endRun()
	assert.Len(t, c.entries, 100)
	assert.Equal(t, 100, c.peak)
}

// sameString reports whether a and b are the same string in memory, rather
// than equal ones.
func sameString(a, b string) bool {
	return len(a) == len(b) && unsafe.StringData(a) == unsafe.StringData(b)
}

func TestHostnameAndTagsReuseStringsAcrossRuns(t *testing.T) {
	k := newKSMCheck(core.NewCheckBase(CheckName), &KSMConfig{LabelsMapper: map[string]string{
		"namespace": "kube_namespace",
		"pod":       "pod_name",
	}}, taggerfxmock.SetupFakeTagger(t), nil)
	k.clusterNameRFC1123 = "cluster"
	labelJoiner := newLabelJoiner(nil)
	labels, err := ksmstore.NewLabels(
		[]string{"namespace", "pod", "node", "created_by_kind", "created_by_name"},
		[]string{"default", "app-6768ddc4d-x7k2p", "node-1", "ReplicaSet", "app-6768ddc4d"},
	)
	require.NoError(t, err)

	hostname1, tags1 := k.hostnameAndTags("kube_pod_info", labels, labelJoiner, nil)
	k.endStringCachesRun()

	hostname2, tags2 := k.hostnameAndTags("kube_pod_info", labels, labelJoiner, nil)
	k.endStringCachesRun()

	assert.Equal(t, "node-1-cluster", hostname1)
	assert.True(t, sameString(hostname1, hostname2), "the hostname is built again")
	assert.ElementsMatch(t, []string{
		"kube_namespace:default",
		"pod_name:app-6768ddc4d-x7k2p",
		"node:node-1",
		"kube_replica_set:app-6768ddc4d",
		"kube_deployment:app",
	}, tags1)
	require.Equal(t, tags1, tags2)
	for i := range tags1 {
		assert.True(t, sameString(tags1[i], tags2[i]), "tag %q is built again", tags1[i])
	}
}

func TestDDMetricName(t *testing.T) {
	k := &KSMCheck{metricNamesMapper: map[string]string{
		"kube_pod_info":          "pod.info",
		"kube_customresource_cr": "customresource.cr",
	}}

	name, found := k.ddMetricName("kube_pod_info")
	assert.True(t, found)
	assert.Equal(t, "kubernetes_state.pod.info", name)

	name, found = k.ddMetricName("kube_customresource_cr")
	assert.True(t, found)
	assert.Equal(t, "kubernetes_state_customresource.cr", name)

	again, _ := k.ddMetricName("kube_customresource_cr")
	assert.True(t, sameString(name, again))

	_, found = k.ddMetricName("kube_unknown")
	assert.False(t, found)
}

// BenchmarkTagsRetainedAcrossRuns measures the memory kept by the tags of the
// series once the aggregator holds those of a first run, and the check has
// built those of a second run.
func BenchmarkTagsRetainedAcrossRuns(b *testing.B) {
	fakeTagger := taggerfxmock.SetupFakeTagger(b)
	labelJoiner := newLabelJoiner(nil)

	const nbPods = 2000
	var series []ksmstore.Labels
	for i := range nbPods {
		for _, container := range []string{"app", "sidecar"} {
			for _, reason := range []string{"ContainerCreating", "CrashLoopBackOff", "ErrImagePull", "ImagePullBackOff", "CreateContainerConfigError"} {
				labels, err := ksmstore.NewLabels(
					[]string{"namespace", "pod", "uid", "node", "created_by_kind", "created_by_name", "container", "reason"},
					[]string{
						fmt.Sprintf("namespace-%d", i%50),
						fmt.Sprintf("app-6768ddc4d-%05d", i),
						fmt.Sprintf("bec19172-8abf-11ea-8546-%012d", i),
						fmt.Sprintf("node-%d", i%100),
						"ReplicaSet",
						"app-6768ddc4d",
						container,
						reason,
					},
				)
				if err != nil {
					b.Fatal(err)
				}
				series = append(series, labels)
			}
		}
	}

	b.ReportAllocs()
	var retained int64
	for b.Loop() {
		// A new check for every measurement, so that each one starts with
		// empty caches.
		k := newKSMCheck(core.NewCheckBase(CheckName), &KSMConfig{LabelsMapper: map[string]string{}}, fakeTagger, nil)
		k.clusterNameRFC1123 = "cluster"

		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)

		// The aggregator keeps the hostname and the tags of the first run
		aggregator := make([][]string, len(series))
		hostnames := make([]string, len(series))
		for i, labels := range series {
			hostnames[i], aggregator[i] = k.hostnameAndTags("kube_pod_container_status_waiting_reason", labels, labelJoiner, nil)
		}
		k.endStringCachesRun()
		// The second run builds them again, keeping its cache
		for _, labels := range series {
			_, _ = k.hostnameAndTags("kube_pod_container_status_waiting_reason", labels, labelJoiner, nil)
		}
		k.endStringCachesRun()

		runtime.GC()
		runtime.ReadMemStats(&after)
		retained = int64(after.HeapAlloc) - int64(before.HeapAlloc)
		runtime.KeepAlive(aggregator)
		runtime.KeepAlive(hostnames)
		runtime.KeepAlive(k)
	}
	b.ReportMetric(float64(retained)/float64(len(series)), "retained-B/series")
	b.ReportMetric(float64(len(series)), "series")
}
