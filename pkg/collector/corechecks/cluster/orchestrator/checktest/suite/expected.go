// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package suite

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Payloads are normalized payloads, by object: "live <uid>" or "deleted <uid>".
type Payloads map[string][]map[string]any

// Live returns the live objects of the UIDs, like "live <uid>".
func Live(uids ...string) []string { return withState("live", uids) }

// Deleted returns the deleted objects of the UIDs, like "deleted <uid>".
func Deleted(uids ...string) []string { return withState("deleted", uids) }

func withState(state string, uids []string) []string {
	objects := make([]string, 0, len(uids))
	for _, uid := range uids {
		objects = append(objects, state+" "+uid)
	}
	return objects
}

// AssertObjects compares the objects that the check sent with the expected ones, like "live <uid>", and fails when it
// sent a payload of an object twice.
func AssertObjects(t *testing.T, expected []string, actual Output) {
	t.Helper()
	if diff := cmp.Diff(sortedCopy(expected), actual.Objects()); diff != "" {
		t.Errorf("sent objects (-want +got):\n%s", diff)
	}
	if duplicates := actual.duplicates(); len(duplicates) > 0 {
		t.Errorf("the check sent these payloads more than once: %v", duplicates)
	}
}

// AssertPayloads compares the payloads that the check sent for the objects of expected with expected. It does not
// check the other objects: AssertObjects does.
func AssertPayloads(t *testing.T, expected Payloads, actual Output) {
	t.Helper()
	got := actual.Payloads(t)
	maps.DeleteFunc(got, func(object string, _ []map[string]any) bool { _, found := expected[object]; return !found })
	for object := range expected {
		if _, found := got[object]; !found {
			got[object] = nil
		}
	}
	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("payloads (-want +got):\n%s", diff)
	}
}

// AssertPayloadFiles compares the payloads that the check sent for the objects with their test data files.
func AssertPayloadFiles(t *testing.T, actual Output, objects ...string) {
	t.Helper()
	AssertPayloads(t, ReadPayloads(t, objects...), actual)
}

// WritePayloadFiles writes the payloads that the check sent for the objects in their test data files, which the
// -update flag of the tests does instead of comparing them.
func WritePayloadFiles(t *testing.T, actual Output, objects ...string) {
	t.Helper()
	got := actual.Payloads(t)
	for _, object := range objects {
		if len(got[object]) == 0 {
			t.Errorf("the check did not send %s, so -update cannot record its payloads", object)
			continue
		}
		state, uid, _ := strings.Cut(object, " ")
		writePayloads(t, uid, state, got[object])
	}
}

// ReadPayloads returns the payloads of the objects from their test data files.
func ReadPayloads(t *testing.T, objects ...string) Payloads {
	t.Helper()
	payloads := Payloads{}
	for _, object := range objects {
		state, uid, _ := strings.Cut(object, " ")
		payloads[object] = readPayloads(t, uid, state)
	}
	return payloads
}

// Objects returns the objects of the payloads, for Env.Pop to wait for.
func (p Payloads) Objects() []string {
	return slices.Sorted(maps.Keys(p))
}

// WithMessageTags returns a copy of the payloads, with the tags added to the message tags of each payload.
func (p Payloads) WithMessageTags(tags ...string) Payloads {
	tagged := Payloads{}
	for object, payloads := range p {
		for _, payload := range payloads {
			payload = maps.Clone(payload)
			messageTags, _ := payload["messageTags"].([]any)
			messageTags = slices.Clone(messageTags)
			for _, tag := range tags {
				messageTags = append(messageTags, tag)
			}
			slices.SortFunc(messageTags, func(a, b any) int { return strings.Compare(a.(string), b.(string)) })
			payload["messageTags"] = messageTags
			tagged[object] = append(tagged[object], payload)
		}
	}
	return tagged
}

// Walk returns a copy of the payloads where fn replaces each value of each map, given its key, like the value of an
// environment variable. A test states this way how its payloads differ from the test data files.
func (p Payloads) Walk(fn func(key string, value any) any) Payloads {
	walked := Payloads{}
	for object, payloads := range p {
		for _, payload := range payloads {
			tagged, _ := walk("", payload, fn).(map[string]any)
			walked[object] = append(walked[object], tagged)
		}
	}
	return walked
}

func walk(key string, value any, fn func(string, any) any) any {
	switch v := value.(type) {
	case map[string]any:
		copied := make(map[string]any, len(v))
		for k, field := range v {
			copied[k] = walk(k, field, fn)
		}
		value = copied
	case []any:
		copied := make([]any, len(v))
		for i, item := range v {
			copied[i] = walk(key, item, fn)
		}
		return fn(key, copied)
	}
	return fn(key, value)
}

// AssertUpdated checks that every payload of an object, except the cluster, has the label that Updated adds.
func AssertUpdated(t *testing.T, actual Output) {
	t.Helper()
	for _, s := range actual {
		if s.ID() == "cluster" {
			continue
		}
		labels := labelsOf(t, s)
		assert.True(t, slices.Contains(labels, "updated:true") || slices.Contains(labels, "updated=true"),
			"%s %s has no label updated: %v", s.Object(), s.Envelope, labels)
	}
}

// AssertDeleted checks that every payload of an object, except the cluster, reports a deleted object, with a deletion
// timestamp.
func AssertDeleted(t *testing.T, actual Output) {
	t.Helper()
	for _, s := range actual {
		if s.ID() == "cluster" {
			continue
		}
		assert.True(t, s.Terminated, "%s %s is not terminated", s.Object(), s.Envelope)
		assert.NotNil(t, metadataOf(t, s)["deletionTimestamp"], "%s %s has no deletion timestamp", s.Object(), s.Envelope)
	}
}

// metadataOf returns the object metadata of a payload: the metadata of the manifest content, or of the metadata model.
func metadataOf(t *testing.T, s Sent) map[string]any {
	model, _ := normalize(t, s)["model"].(map[string]any)
	if content, ok := model["content"].(map[string]any); ok {
		model = content
	}
	metadata, _ := model["metadata"].(map[string]any)
	return metadata
}

// labelsOf returns the labels of a payload, as key:value for a metadata model and key=value for a manifest.
func labelsOf(t *testing.T, s Sent) []string {
	var labels []string
	switch l := metadataOf(t, s)["labels"].(type) {
	case []any:
		for _, label := range l {
			labels = append(labels, label.(string))
		}
	case map[string]any:
		for key, value := range l {
			labels = append(labels, key+"="+value.(string))
		}
	}
	return labels
}

// readPayloads returns the payloads of an object state from the section of the object in its test data file.
func readPayloads(t *testing.T, uid, state string) []map[string]any {
	t.Helper()
	path := testDataPath(uid)
	sections, err := readSections(path)
	require.NoError(t, err)
	raw, found := sections[uid][state]
	if !found {
		t.Fatalf("%s has no %s payloads of %s: run the test with -update", path, state, uid)
	}
	var payloads []map[string]any
	require.NoError(t, json.Unmarshal(raw, &payloads), "%s: %s %s", path, state, uid)
	return payloads
}

var (
	testDataMu sync.Mutex
	// written are the payloads that this run wrote with -update, by object, so that tests that check the same
	// payloads send the same.
	written = map[string]string{}
)

// writePayloads writes the payloads of an object state in the section of the object in its test data file, and keeps
// the rest of the file.
func writePayloads(t *testing.T, uid, state string, payloads []map[string]any) {
	t.Helper()
	testDataMu.Lock()
	defer testDataMu.Unlock()
	data, err := json.Marshal(payloads)
	require.NoError(t, err)
	object := state + " " + uid
	if previous, found := written[object]; found && previous != string(data) {
		t.Errorf("another test wrote other payloads of %s", object)
		return
	}
	written[object] = string(data)
	require.NoError(t, writeSection(uid, state, payloads))
}

// BuiltinUIDs returns the UIDs of the objects of the built-in resources in the default environment, which serves the
// first version of each resource.
func BuiltinUIDs(t *testing.T) []string {
	var uids []string
	for _, f := range servedVersions(builtinFixtures(t), false) {
		uids = append(uids, uidOf(f.Object))
	}
	return uids
}

// CustomResourceUID returns the UID of the object of a built-in custom resource in the default environment.
func CustomResourceUID(collector string) string {
	return objectUID(collector)
}
