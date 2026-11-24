// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package suite

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

// Sent is one resource of a payload that the check sent: a manifest, or a structured metadata model.
type Sent struct {
	Manifest   bool
	Envelope   string
	UID        string
	Terminated bool
	// MessageTags are the tags of the payload.
	MessageTags []string
	// Message are the fields of the payload that identify where it comes from, like the cluster name. The group of a
	// payload is left out, as it counts the payloads of the check.
	Message map[string]any
	// Model is the decoded resource: a *model.Manifest, or the structured metadata model.
	Model any
}

// ID returns the UID of the object, or "cluster" for the cluster.
func (s Sent) ID() string {
	if s.UID == clusterID {
		return "cluster"
	}
	return s.UID
}

// Object returns "live <id>" or "deleted <id>".
func (s Sent) Object() string {
	if s.Terminated {
		return "deleted " + s.ID()
	}
	return "live " + s.ID()
}

// Output is everything that the check sent during a step.
type Output []Sent

// Objects returns the distinct objects of the output, sorted.
func (o Output) Objects() []string {
	var objects []string
	for _, s := range o {
		if !slices.Contains(objects, s.Object()) {
			objects = append(objects, s.Object())
		}
	}
	return sortedCopy(objects)
}

// ManifestContent returns the manifest content of an object of the output, like "live <uid>", as JSON values. The test
// fails unless the output has one manifest of the object.
func (o Output) ManifestContent(t *testing.T, object string) map[string]any {
	t.Helper()
	var contents []map[string]any
	for _, payload := range o.Payloads(t)[object] {
		if payload["type"] == "manifest" {
			model, _ := payload["model"].(map[string]any)
			content, _ := model["content"].(map[string]any)
			contents = append(contents, content)
		}
	}
	require.Len(t, contents, 1, "manifests of %s", object)
	return contents[0]
}

// Payloads returns the normalized payloads of each object of the output.
func (o Output) Payloads(t testing.TB) Payloads {
	payloads := Payloads{}
	for _, s := range o {
		payloads[s.Object()] = append(payloads[s.Object()], normalize(t, s))
	}
	for _, values := range payloads {
		slices.SortFunc(values, func(a, b map[string]any) int { return strings.Compare(fmt.Sprint(a), fmt.Sprint(b)) })
	}
	return payloads
}

// deletionTime replaces the deletion timestamp that the check sets, at the time of the run.
const deletionTime = "<deletion time>"

// podVersion replaces the version of a pod, a hash of its model, which a test cannot compute after it changes a field.
const podVersion = "<hash of the pod model>"

// managedFields replaces the managed fields of a manifest.
const managedFields = "<managed fields>"

// normalize returns a payload as JSON values, with the manifest content decoded so that a difference names the field,
// the lists that the check builds from maps sorted, and the deletion time replaced.
func normalize(t testing.TB, s Sent) map[string]any {
	value := map[string]any{"envelope": s.Envelope, "messageTags": sortedCopy(s.MessageTags), "message": s.Message}
	m := jsonValue(t, s.Model)
	if manifest, ok := s.Model.(*model.Manifest); ok {
		value["type"] = "manifest"
		if len(manifest.Content) > 0 {
			var content map[string]any
			require.NoError(t, json.Unmarshal(manifest.Content, &content), "%s: manifest content", s.UID)
			if metadata, ok := content["metadata"].(map[string]any); ok {
				if metadata["deletionTimestamp"] != nil {
					metadata["deletionTimestamp"] = deletionTime
				}
				// The fake clients generate the managed fields of each change. The golden files only show that they are
				// sent, and assertNoSensitiveData checks the kinds that must not send them.
				if metadata["managedFields"] != nil {
					metadata["managedFields"] = managedFields
				}
			}
			m["content"] = content
		}
	} else {
		value["type"] = "metadata"
		if metadata, ok := m["metadata"].(map[string]any); ok && metadata["deletionTimestamp"] != nil {
			metadata["deletionTimestamp"] = deletionTime
		}
	}
	if s.Envelope == "CollectorPod" || m["kind"] == "Pod" {
		// The version of a pod is a hash of its model, which the payload holds, with the deletion time.
		m["resourceVersion"] = podVersion
		if metadata, ok := m["metadata"].(map[string]any); ok {
			metadata["resourceVersion"] = podVersion
		}
	}
	sortUnorderedLists(m)
	value["model"] = m
	// The round trip gives the values the types that a golden file decodes to.
	return jsonValue(t, value)
}

func jsonValue(t testing.TB, v any) map[string]any {
	data, err := json.Marshal(v)
	require.NoError(t, err)
	var value map[string]any
	require.NoError(t, json.Unmarshal(data, &value))
	return value
}

// unorderedLists are the lists that the check builds from maps, in a random order.
var unorderedLists = []string{"tags", "labels", "annotations"}

func sortUnorderedLists(value any) {
	switch v := value.(type) {
	case map[string]any:
		for key, field := range v {
			if list, ok := field.([]any); ok && slices.Contains(unorderedLists, key) {
				slices.SortFunc(list, func(a, b any) int { return strings.Compare(fmt.Sprint(a), fmt.Sprint(b)) })
				continue
			}
			sortUnorderedLists(field)
		}
	case []any:
		for _, item := range v {
			sortUnorderedLists(item)
		}
	}
}

// assertNoSensitiveData checks that no payload holds the "secret" of the fixtures, unless a test turns container
// scrubbing off, and that ConfigMaps and Secrets send no managed fields.
func (env *Env) assertNoSensitiveData(t *testing.T, out Output) {
	t.Helper()
	scrubbing := env.Config.GetBool("orchestrator_explorer.container_scrubbing.enabled")
	for _, s := range out {
		manifest, ok := s.Model.(*model.Manifest)
		if !ok {
			if scrubbing {
				data, err := json.Marshal(s.Model)
				require.NoError(t, err)
				assert.False(t, bytes.Contains(data, []byte(`"secret"`)), "%s %s sends a sensitive value: %s", s.Object(), s.Envelope, data)
				assert.False(t, bytes.Contains(yamlOf(s.Model), []byte("secret")), "%s %s sends a sensitive value in its YAML", s.Object(), s.Envelope)
			}
			continue
		}
		if scrubbing {
			assert.False(t, bytes.Contains(manifest.Content, []byte(`"secret"`)), "%s sends a sensitive value: %s", s.Object(), manifest.Content)
		}
		if manifest.Kind == "ConfigMap" || manifest.Kind == "Secret" {
			assert.False(t, bytes.Contains(manifest.Content, []byte("managedFields")), "%s sends managed fields", s.Object())
		}
	}
}

// yamlOf returns the YAML of the object that a structured metadata model holds, if it has one.
func yamlOf(model any) []byte {
	value := reflect.ValueOf(model)
	if value.Kind() != reflect.Pointer || value.Elem().Kind() != reflect.Struct {
		return nil
	}
	if field := value.Elem().FieldByName("Yaml"); field.IsValid() && field.Kind() == reflect.Slice {
		return field.Bytes()
	}
	return nil
}

// duplicates returns the payloads of the output that the check sent more than once: the same object, payload type and
// state.
func (o Output) duplicates() []string {
	counts := map[string]int{}
	for _, s := range o {
		counts[s.Object()+" "+s.Envelope]++
	}
	var duplicates []string
	for key, count := range counts {
		if count > 1 {
			duplicates = append(duplicates, key)
		}
	}
	return sortedCopy(duplicates)
}

// recorder records the resources of the payloads that the pipelines send, from any goroutine.
type recorder struct {
	sender *mocksender.MockSender
	mu     sync.Mutex
	sent   Output
}

func newRecorder(t *testing.T, id checkid.ID, senderManager sender.SenderManager) *recorder {
	r := &recorder{sender: mocksender.NewMockSenderWithSenderManager(id, senderManager)}
	r.sender.On("OrchestratorMetadata", mock.Anything, mock.Anything, mock.Anything).Return().Run(func(args mock.Arguments) {
		r.record(metadataResources(args.Get(0).([]model.MessageBody)))
	})
	r.sender.On("OrchestratorManifest", mock.Anything, mock.Anything).Return().Run(func(args mock.Arguments) {
		r.record(manifestResources(t, args.Get(0).([]model.MessageBody)))
	})
	return r
}

func (r *recorder) record(resources Output) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, resources...)
}

// take returns the resources sent since the last call.
func (r *recorder) take() Output {
	r.mu.Lock()
	defer r.mu.Unlock()
	sent := r.sent
	r.sent = nil
	return sent
}

func manifestResources(t *testing.T, messages []model.MessageBody) Output {
	var resources Output
	for _, message := range messages {
		var envelope string
		var manifests *model.CollectorManifest
		switch m := message.(type) {
		case *model.CollectorManifest:
			envelope, manifests = "CollectorManifest", m
		case *model.CollectorManifestCR:
			envelope, manifests = "CollectorManifestCR", m.Manifest
		case *model.CollectorManifestCRD:
			envelope, manifests = "CollectorManifestCRD", m.Manifest
		default:
			t.Errorf("unknown manifest payload %T", message)
			continue
		}
		for _, manifest := range manifests.Manifests {
			resources = append(resources, Sent{Manifest: true, Envelope: envelope, UID: manifest.Uid, Terminated: manifest.IsTerminated,
				MessageTags: manifests.Tags, Message: messageFields(manifests), Model: manifest})
		}
	}
	return resources
}

// metadataResources returns the resources of structured metadata payloads. Each payload type holds a list of models
// with object metadata, except the cluster.
func metadataResources(messages []model.MessageBody) Output {
	var resources Output
	for _, message := range messages {
		if cluster, ok := message.(*model.CollectorCluster); ok {
			resources = append(resources, Sent{Envelope: "CollectorCluster", UID: cluster.ClusterId, MessageTags: cluster.Tags,
				Message: messageFields(cluster), Model: cluster.Cluster})
			continue
		}
		payload := reflect.ValueOf(message).Elem()
		var messageTags []string
		if tagsField := payload.FieldByName("Tags"); tagsField.IsValid() {
			messageTags, _ = tagsField.Interface().([]string)
		}
		envelope := strings.TrimPrefix(fmt.Sprintf("%T", message), "*process.")
		for i := range payload.NumField() {
			field := payload.Field(i)
			if field.Kind() != reflect.Slice || field.Type().Elem().Kind() != reflect.Pointer || field.Type().Elem().Elem().Kind() != reflect.Struct {
				continue
			}
			for j := range field.Len() {
				item := field.Index(j).Elem()
				metadataField := item.FieldByName("Metadata")
				if !metadataField.IsValid() {
					continue
				}
				if objectMetadata, ok := metadataField.Interface().(*model.Metadata); ok && objectMetadata != nil {
					resources = append(resources, Sent{Envelope: envelope, UID: objectMetadata.Uid, Terminated: objectMetadata.DeletionTimestamp != 0,
						MessageTags: messageTags, Message: messageFields(message), Model: item.Addr().Interface()})
				}
			}
		}
	}
	return resources
}

// messageFieldNames are the fields of a payload that identify where it comes from.
var messageFieldNames = []string{"ClusterName", "ClusterId", "HostName", "OriginCollector"}

// messageFields returns the fields of a payload that identify where it comes from.
func messageFields(message any) map[string]any {
	payload := reflect.ValueOf(message).Elem()
	fields := map[string]any{}
	for _, name := range messageFieldNames {
		if field := payload.FieldByName(name); field.IsValid() {
			fields[name] = fmt.Sprint(field.Interface())
		}
	}
	return fields
}
