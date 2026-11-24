// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package suite

import (
	"errors"
	"flag"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// update makes AssertPayloadFiles write the payload files with what the check sends, instead of comparing them.
var update = flag.Bool("update", false, "write the payload files with what the check sends")

// PayloadDir holds the payloads that the check sends for each object of the default environment, relative to the
// package of the tests. The file of an object is its UID, like the file of its manifest in ManifestDir: it holds its
// live payloads, and its deleted payloads when a test checks them.
var PayloadDir = filepath.Join("testdata", "payloads")

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

// AssertPayloadFiles compares the payloads that the check sent for the objects with their payload files. With
// -update, it writes them in the files instead.
func AssertPayloadFiles(t *testing.T, actual Output, objects ...string) {
	t.Helper()
	if !*update {
		AssertPayloads(t, ReadPayloads(t, objects...), actual)
		return
	}
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

// ReadPayloads returns the payloads of the objects from their payload files.
func ReadPayloads(t *testing.T, objects ...string) Payloads {
	t.Helper()
	payloads := Payloads{}
	for _, object := range objects {
		state, uid, _ := strings.Cut(object, " ")
		byState := readPayloadFile(t, uid)
		if _, found := byState[state]; !found {
			t.Fatalf("%s has no %s payloads: run the test with -update", payloadPath(uid), state)
		}
		payloads[object] = byState[state]
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
// environment variable. A test states this way how its payloads differ from the payload files.
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

func payloadPath(uid string) string { return filepath.Join(PayloadDir, uid+".yaml") }

// readPayloadFile returns the payloads of a payload file, by object state.
func readPayloadFile(t *testing.T, uid string) map[string][]map[string]any {
	t.Helper()
	data, err := os.ReadFile(payloadPath(uid))
	if errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("no payload file %s: run the test with -update", payloadPath(uid))
	}
	require.NoError(t, err)
	var byState map[string][]map[string]any
	require.NoError(t, yaml.Unmarshal(data, &byState), payloadPath(uid))
	return byState
}

var (
	payloadFilesMu sync.Mutex
	// written are the payloads that this run wrote with -update, by file and state, so that tests that check the
	// same payloads send the same.
	written = map[string]string{}
)

// writePayloads writes the payloads of an object state in its payload file, and keeps its other states.
func writePayloads(t *testing.T, uid, state string, payloads []map[string]any) {
	t.Helper()
	payloadFilesMu.Lock()
	defer payloadFilesMu.Unlock()
	path := payloadPath(uid)
	data, err := yaml.Marshal(payloads)
	require.NoError(t, err)
	if previous, found := written[path+" "+state]; found && previous != string(data) {
		t.Errorf("another test wrote other %s payloads to %s", state, path)
		return
	}
	written[path+" "+state] = string(data)

	byState := map[string][]map[string]any{}
	if existing, err := os.ReadFile(path); err == nil {
		require.NoError(t, yaml.Unmarshal(existing, &byState), path)
	}
	byState[state] = payloads
	data, err = yaml.Marshal(byState)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o644))
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
