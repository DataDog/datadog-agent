// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package suite

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
)

// ObjectSet selects objects of the environment, when a step runs.
type ObjectSet func(env *Env) []*Object

var (
	// Collected are the objects that the check collects.
	Collected ObjectSet = (*Env).Collected
	// Originals are the collected objects that CopyAll did not add.
	Originals ObjectSet = func(env *Env) []*Object { return filter(env.Collected(), func(o *Object) bool { return !o.Copy }) }
	// Copies are the collected objects that CopyAll added.
	Copies ObjectSet = func(env *Env) []*Object { return filter(env.Collected(), func(o *Object) bool { return o.Copy }) }
	// CustomResources are the collected custom resources.
	CustomResources ObjectSet = func(env *Env) []*Object {
		return filter(env.Collected(), func(o *Object) bool { return o.Resource.IsCustomResource() })
	}
)

func filter(objects []*Object, keep func(*Object) bool) []*Object {
	var kept []*Object
	for _, o := range objects {
		if keep(o) {
			kept = append(kept, o)
		}
	}
	return kept
}

// NoObjects expects the check to send no object. The runner still expects the cluster when the nodes changed.
func NoObjects(*testing.T, *Env) Payloads { return Payloads{} }

// Live expects the live payloads of the objects, in their current state.
func Live(objects ObjectSet) func(*testing.T, *Env) Payloads {
	return func(t *testing.T, env *Env) Payloads { return env.liveOf(t, objects(env)) }
}

// Deleted expects the payloads of the deletion of the objects, for those whose deletion the check reports.
func Deleted(objects ObjectSet) func(*testing.T, *Env) Payloads {
	return func(t *testing.T, env *Env) Payloads { return env.terminatedOf(t, objects(env)) }
}

// UpdateAll adds a label to the objects, with a new resource version.
func UpdateAll(objects ObjectSet) func(*testing.T, *Env) {
	return func(t *testing.T, env *Env) {
		for _, o := range objects(env) {
			updated := o.State.DeepCopyObject()
			accessor, err := meta.Accessor(updated)
			require.NoError(t, err)
			version, err := strconv.Atoi(accessor.GetResourceVersion())
			require.NoError(t, err)
			accessor.SetResourceVersion(strconv.Itoa(version + 1))
			accessor.SetLabels(map[string]string{"updated": "true"})
			env.update(t, updated)
		}
	}
}

// CopyAll adds a copy of each object to the cluster, with its own name and UID.
func CopyAll(objects ObjectSet) func(*testing.T, *Env) {
	return func(t *testing.T, env *Env) { env.addCopies(t, objects(env)) }
}

// DeleteAll deletes the objects, and waits until the check has queued the deletions that it reports.
func DeleteAll(objects ObjectSet) func(*testing.T, *Env) {
	return func(t *testing.T, env *Env) { env.deleteAll(t, objects(env)) }
}

// SetLeader sets the result of the leader election of the next runs.
func SetLeader(leader bool) func(*testing.T, *Env) {
	return func(_ *testing.T, env *Env) { env.leader = leader }
}

// FailDiscovery makes API discovery return the error, or work again when it is nil.
func FailDiscovery(err error) func(*testing.T, *Env) {
	return func(_ *testing.T, env *Env) { env.discovery.fail(err) }
}
