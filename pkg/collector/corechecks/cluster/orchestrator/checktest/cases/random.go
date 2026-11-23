// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package cases

import (
	"encoding/json"
	"fmt"
	"maps"
	"math/rand"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/checktest/suite"
)

func init() { suite.Register("RandomEvents", randomEvents) }

// The random scenarios apply seeded sequences of object creations, updates and deletions, leadership changes and check
// runs, and check what the check sent against a model of the cluster each time the informers settle.

// randomObject is the model of an object that the random scenario changes.
type randomObject struct {
	FullName string
	Object   runtime.Object
	Metadata bool
	Present  bool
	// Deletes reports whether the check sends the deletion of the object.
	Deletes bool
	// TerminatedExpected reports whether the deletion of the object must be sent, once it is deleted.
	TerminatedExpected bool
}

// randomEvents runs seeded sequences of random events.
func randomEvents(t *testing.T, cs *suite.Suite) {
	for seed := int64(1); seed <= 8; seed++ {
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) { RunRandomScenario(t, cs.SenderManager, seed) })
	}
}

func RunRandomScenario(t *testing.T, senderManager sender.SenderManager, seed int64) {
	random := rand.New(rand.NewSource(seed))
	cfg := suite.ScenarioConfig{DedicatedPipeline: random.Intn(2) == 0}
	t.Logf("seed %d: %s", seed, cfg)
	sc := suite.NewScenario(t, senderManager, suite.ClusterAgentLeader, cfg)

	// The random events change every object that the check collects.
	model := map[types.UID]*randomObject{}
	var targets []string
	for _, o := range sc.Collected() {
		e := sc.Expected.Live[o.UID()]
		model[types.UID(o.UID())] = &randomObject{FullName: o.FullName, Object: o.Object, Metadata: o.Metadata, Present: true,
			Deletes: e.Deletes || e.FullName == "v1/pods"}
		targets = append(targets, o.FullName)
	}
	leader := true
	var history []suite.SentResource
	collect := func() {
		sent := sc.Run()
		if !leader {
			assert.Empty(t, sent, "a follower sends nothing")
		}
		history = append(history, sent...)
	}
	collect()

	created := 0
	for i := range 40 {
		switch op := random.Intn(20); {
		case op < 6: // update
			if o := randomPresent(random, model); o != nil {
				updated := o.Object.DeepCopyObject()
				accessor, _ := meta.Accessor(updated)
				version, _ := strconv.Atoi(accessor.GetResourceVersion())
				accessor.SetResourceVersion(strconv.Itoa(version + 1))
				accessor.SetLabels(map[string]string{"step": strconv.Itoa(i)})
				sc.Mocks.Cluster.Update(t, updated)
				o.Object = updated
			}
		case op < 9: // create
			created++
			source := model[types.UID(sc.Object(t, targets[random.Intn(len(targets))]).UID())]
			object := source.Object.DeepCopyObject()
			accessor, _ := meta.Accessor(object)
			accessor.SetUID(types.UID(fmt.Sprintf("random-%d", created)))
			accessor.SetName(fmt.Sprintf("random-%d", created))
			accessor.SetResourceVersion("1")
			sc.Mocks.Cluster.Add(t, object)
			model[accessor.GetUID()] = &randomObject{FullName: source.FullName, Object: object, Metadata: source.Metadata, Present: true, Deletes: source.Deletes}
		case op < 13: // delete
			if o := randomPresent(random, model); o != nil {
				sc.Mocks.Cluster.Delete(t, o.Object)
				o.Present = false
				o.TerminatedExpected = leader && o.Deletes
			}
		case op < 18:
			collect()
		default: // leadership change, once the informers reported every change
			settle(t, sc, targets, model)
			collect()
			leader = !leader
			sc.Mocks.Leader = leader
			collect()
		}
	}

	if !leader {
		settle(t, sc, targets, model)
		leader = true
		sc.Mocks.Leader = true
	}
	settle(t, sc, targets, model)
	for range 3 {
		collect()
	}
	assertRandomHistory(t, model, history)
}

func randomPresent(random *rand.Rand, model map[types.UID]*randomObject) *randomObject {
	var present []*randomObject
	for _, uid := range sortedUIDs(model) {
		if model[uid].Present {
			present = append(present, model[uid])
		}
	}
	if len(present) == 0 {
		return nil
	}
	return present[random.Intn(len(present))]
}

func sortedUIDs(model map[types.UID]*randomObject) []types.UID {
	uids := slices.Collect(maps.Keys(model))
	slices.Sort(uids)
	return uids
}

// settle waits until the informers of the changed collectors hold the objects of the model, and lets their delete
// handlers run.
func settle(t *testing.T, sc *suite.Scenario, targets []string, model map[types.UID]*randomObject) {
	t.Helper()
	require.Eventually(t, func() bool {
		for _, fullName := range slices.Compact(slices.Sorted(slices.Values(targets))) {
			expected := map[types.UID]string{}
			for uid, o := range model {
				if o.FullName == fullName && o.Present {
					accessor, _ := meta.Accessor(o.Object)
					expected[uid] = accessor.GetResourceVersion()
				}
			}
			for _, collector := range sc.Collectors(fullName) {
				stored := map[types.UID]string{}
				for _, object := range collector.Informer().GetStore().List() {
					if accessor, err := meta.Accessor(object); err == nil {
						if _, tracked := model[accessor.GetUID()]; tracked {
							stored[accessor.GetUID()] = accessor.GetResourceVersion()
						}
					}
				}
				if fmt.Sprint(stored) != fmt.Sprint(expected) {
					return false
				}
			}
		}
		return true
	}, 5*time.Second, 5*time.Millisecond)
	time.Sleep(30 * time.Millisecond)
}

// assertRandomHistory checks everything that the check sent against the final model: each version of an object is sent
// once, the last version of each object that exists is sent, and the deletion of an object is sent once if it
// happened while the check was the leader, and never otherwise.
func assertRandomHistory(t *testing.T, model map[types.UID]*randomObject, history []suite.SentResource) {
	t.Helper()
	sentKeys := map[string]int{}
	// The payload version of a pod is a hash of its model, so the manifest content gives the version of the object.
	lastVersion := map[string]map[bool]string{}
	lastObjectVersion := map[string]string{}
	terminated := map[types.UID]map[bool]int{}
	for _, s := range history {
		uid := types.UID(s.UID)
		if _, tracked := model[uid]; !tracked {
			continue
		}
		sentKeys[fmt.Sprintf("%s/%s/metadata=%t/terminated=%t", s.UID, s.ResourceVersion, s.Metadata, s.Terminated)]++
		if s.Terminated {
			if terminated[uid] == nil {
				terminated[uid] = map[bool]int{}
			}
			terminated[uid][s.Metadata]++
			continue
		}
		if lastVersion[s.UID] == nil {
			lastVersion[s.UID] = map[bool]string{}
		}
		lastVersion[s.UID][s.Metadata] = s.ResourceVersion
		if !s.Metadata {
			var content struct {
				Metadata struct {
					ResourceVersion string `json:"resourceVersion"`
				} `json:"metadata"`
			}
			require.NoError(t, json.Unmarshal(s.Content, &content), "%s: manifest content", s.UID)
			lastObjectVersion[s.UID] = content.Metadata.ResourceVersion
		}
	}
	for key, count := range sentKeys {
		assert.Equal(t, 1, count, "%s is sent more than once", key)
	}

	for _, uid := range sortedUIDs(model) {
		o := model[uid]
		accessor, _ := meta.Accessor(o.Object)
		if o.Present {
			assert.Equal(t, accessor.GetResourceVersion(), lastObjectVersion[string(uid)], "%s %s: the last version is sent", o.FullName, uid)
			if o.Metadata {
				assert.Equal(t, lastVersion[string(uid)][false], lastVersion[string(uid)][true],
					"%s %s: the metadata model of the last version is sent", o.FullName, uid)
			}
		}
		payloads := map[bool]bool{true: o.Metadata, false: true}
		for metadata, expected := range payloads {
			if !expected {
				continue
			}
			if o.Present {
				assert.Zero(t, terminated[uid][metadata], "%s %s is sent as terminated but exists", o.FullName, uid)
				continue
			}
			want := 0
			if o.TerminatedExpected {
				want = 1
			}
			assert.Equal(t, want, terminated[uid][metadata], "%s %s: terminated payloads, metadata=%t", o.FullName, uid, metadata)
		}
	}
}
