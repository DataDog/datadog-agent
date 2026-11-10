// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package helm

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"math/rand"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

func TestHelmCacheLiveHeap(t *testing.T) {
	// Do not run in parallel: HeapAlloc measures the entire process. Large
	// payloads make the expected difference much larger than runtime noise.
	for _, storage := range []helmStorage{k8sConfigmaps, k8sSecrets} {
		t.Run(string(storage), func(t *testing.T) {
			fixtures := helmCacheMemoryFixtures(t, storage, 20)
			for _, valuesAsTags := range []bool{false, true} {
				t.Run(fmt.Sprintf("values_as_tags=%t", valuesAsTags), func(t *testing.T) {
					before := helmCacheLiveHeap(t, fixtures, storage, false, valuesAsTags)
					after := helmCacheLiveHeap(t, fixtures, storage, true, valuesAsTags)
					t.Logf("%d revisions: before=%d live bytes, after=%d live bytes, reduction=%.2f%%",
						len(fixtures), before, after, 100*(1-float64(after)/float64(before)))
					require.Positive(t, after)
					require.Less(t, after, before/2, "compact caches should release the large payloads")
				})
			}
		})
	}
}

// BenchmarkHelmCacheLiveHeap compares steady-state heap retention, not allocation
// volume. Both cases populate the real client-go indexer and Helm release store.
// "before" retains the full API objects, as the informer did before the change;
// "after" retains transformRelease's compact objects. The release store has the
// same summaries and tags in both cases. This is a synthetic cache comparison,
// not a benchmark of the old decoder's CPU cost or an end-to-end informer run.
//
// Run with -test.run=^$ -test.bench=BenchmarkHelmCacheLiveHeap
// -test.benchtime=1x -test.count=3. Compare live-B/cache and live-B/revision;
// ns/op includes forced GC and is not a throughput measurement.
func BenchmarkHelmCacheLiveHeap(b *testing.B) {
	for _, storage := range []helmStorage{k8sConfigmaps, k8sSecrets} {
		fixtures := helmCacheMemoryFixtures(b, storage, 100)
		for _, valuesAsTags := range []bool{false, true} {
			for _, compact := range []bool{false, true} {
				mode := "before"
				if compact {
					mode = "after"
				}
				b.Run(fmt.Sprintf("%s/values_as_tags=%t/%s", storage, valuesAsTags, mode), func(b *testing.B) {
					var liveBytes int64
					for i := 0; i < b.N; i++ {
						liveBytes += helmCacheLiveHeap(b, fixtures, storage, compact, valuesAsTags)
					}
					perCache := float64(liveBytes) / float64(b.N)
					b.ReportMetric(perCache, "live-B/cache")
					b.ReportMetric(perCache/float64(len(fixtures)), "live-B/revision")
				})
			}
		}
	}
}

func helmCacheLiveHeap(tb testing.TB, fixtures [][]byte, storage helmStorage, compact, valuesAsTags bool) int64 {
	tb.Helper()
	check := newCheck().(*HelmCheck)
	if valuesAsTags {
		check.instance.HelmValuesAsTags = map[string]string{"option": "option_value"}
	}
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	var before, after runtime.MemStats
	// A second collection also clears pools that can temporarily retain buffers.
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&before)
	populateHelmMemoryCache(tb, fixtures, storage, compact, check, indexer)
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&after)
	// Keep the cache, summary store, and source fixtures alive through the sample.
	// In particular, fixture storage must not disappear between measurements.
	runtime.KeepAlive(indexer)
	runtime.KeepAlive(check)
	runtime.KeepAlive(fixtures)
	return int64(after.HeapAlloc) - int64(before.HeapAlloc)
}

func populateHelmMemoryCache(tb testing.TB, fixtures [][]byte, storage helmStorage, compact bool, check *HelmCheck, indexer cache.Indexer) {
	tb.Helper()
	for _, wire := range fixtures {
		var obj interface{}
		if storage == k8sConfigmaps {
			cm := &v1.ConfigMap{}
			require.NoError(tb, cm.Unmarshal(wire))
			obj = cm
		} else {
			secret := &v1.Secret{}
			require.NoError(tb, secret.Unmarshal(wire))
			obj = secret
		}
		summary, err := check.transformRelease(obj)
		require.NoError(tb, err)
		require.NotNil(tb, summary.(*cachedRelease).taggedRelease)
		check.addRelease(summary)
		if compact {
			obj = summary
		}
		require.NoError(tb, indexer.Add(obj))
	}
	require.Len(tb, indexer.List(), len(fixtures))
	require.Len(tb, check.store.getAll(storage), len(fixtures))
	// Return before collecting so temporary references to raw API objects and
	// decoded values cannot contribute to the retained-heap measurement.
}

func helmCacheMemoryFixtures(tb testing.TB, storage helmStorage, count int) [][]byte {
	tb.Helper()
	// Deterministic, poorly compressible data avoids unrealistically tiny gzip
	// payloads. Each revision carries a 128 KiB manifest and 32 KiB of values.
	rng := rand.New(rand.NewSource(1))
	payload := make([]byte, 160*1024)
	for i := range payload {
		payload[i] = 'a' + byte(rng.Intn(26))
	}
	fixtures := make([][]byte, count)
	for i := range fixtures {
		rel := release{
			Name: fmt.Sprintf("app-%d", i/10), Namespace: "default", Version: i%10 + 1,
			Info:   &info{Status: "deployed"},
			Config: map[string]interface{}{"option": "override", "unused": string(payload[:32*1024])},
			Chart: &chart{
				Metadata: &metadata{Name: "chart", Version: "1.0.0", AppVersion: "1.0.0"},
				Values:   map[string]interface{}{"option": "default"},
			},
		}
		data, err := json.Marshal(struct {
			release
			Manifest string `json:"manifest"`
		}{rel, string(payload[32*1024:])})
		require.NoError(tb, err)
		var compressed bytes.Buffer
		writer := gzip.NewWriter(&compressed)
		_, err = writer.Write(data)
		require.NoError(tb, err)
		require.NoError(tb, writer.Close())
		encoded := b64.EncodeToString(compressed.Bytes())
		metadata := metav1.ObjectMeta{
			Name: fmt.Sprintf("sh.helm.release.v1.%s.v%d", rel.Name, rel.Version), Namespace: rel.Namespace,
			ResourceVersion: strconv.Itoa(i + 1), Labels: map[string]string{"owner": "helm"},
		}
		if storage == k8sConfigmaps {
			fixtures[i], err = (&v1.ConfigMap{ObjectMeta: metadata, Data: map[string]string{"release": encoded}}).Marshal()
		} else {
			fixtures[i], err = (&v1.Secret{ObjectMeta: metadata, Data: map[string][]byte{"release": []byte(encoded)}}).Marshal()
		}
		require.NoError(tb, err)
	}
	return fixtures
}
