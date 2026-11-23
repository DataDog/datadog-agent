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
	"slices"
	"strings"
	"sync"
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
)

// Payloads are the payloads of each resource: its whole manifest with the content decoded, and the message tags of its
// metadata model, whose fields the transformer tests check.
type Payloads map[payloadKey][]map[string]any

// payloadKey identifies the payloads of a resource: its manifest or its metadata model, live or terminated.
type payloadKey struct {
	UID                  string
	Metadata, Terminated bool
}

func (p Payloads) add(key payloadKey, value map[string]any) {
	p[key] = append(p[key], value)
}

// With returns the payloads of p and of the others.
func (p Payloads) With(others ...Payloads) Payloads {
	all := Payloads{}
	for _, payloads := range append([]Payloads{p}, others...) {
		for key, values := range payloads {
			all[key] = append(all[key], values...)
		}
	}
	return all
}

// WithMessageTags adds tags to the message of every payload.
func (p Payloads) WithMessageTags(tags ...string) Payloads {
	for _, values := range p {
		for _, value := range values {
			value["messageTags"] = sortedTags(append(slices.Clone(value["messageTags"].([]string)), tags...))
		}
	}
	return p
}

// receivedPayloads returns the whole payloads of the sent resources, by resource.
func receivedPayloads(t *testing.T, sent []Sent, ignore func(Sent) bool) Payloads {
	received := Payloads{}
	for _, s := range sent {
		if ignore != nil && ignore(s) {
			continue
		}
		key := payloadKey{s.UID, s.Metadata, s.Terminated}
		if s.Metadata {
			value := map[string]any{"messageTags": sortedTags(s.MessageTags)}
			if len(s.Content) > 0 {
				value["yaml"] = string(s.Content)
			}
			received.add(key, value)
			continue
		}
		manifest, ok := s.Model.(*model.Manifest)
		require.True(t, ok, "%s: the manifest model is a %T", s.UID, s.Model)
		received.add(key, manifestValue(t, manifest, s.Envelope, s.MessageTags))
	}
	return received
}

// liveOf returns the payloads of the objects, in their current state.
func (env *Env) liveOf(t *testing.T, objects []*Object) Payloads {
	all := Payloads{}
	for _, o := range objects {
		all = all.With(env.PayloadsOf(t, o.FullName, o.State, false))
	}
	return all
}

// terminatedOf returns the payloads of the deletion of the objects whose deletion the check reports, in their last
// state.
func (env *Env) terminatedOf(t *testing.T, objects []*Object) Payloads {
	all := Payloads{}
	for _, o := range objects {
		if env.reportsDeletion(o) {
			all = all.With(env.PayloadsOf(t, o.FullName, o.State, true))
		}
	}
	return all
}

// PayloadsOf returns what the check sends for an object of the collector: its whole manifest, and a metadata model if
// its kind has one.
func (env *Env) PayloadsOf(t *testing.T, fullName string, object runtime.Object, terminated bool) Payloads {
	resource := resourceNamed(t, fullName)
	manifest := baseManifest(t, resource, fullName, object, terminated)
	payloads := Payloads{}
	payloads.add(payloadKey{manifest.Uid, false, terminated}, manifestValue(t, manifest, manifestEnvelope(resource), nil))
	if hasMetadata(resource, fullName) {
		payloads.add(payloadKey{manifest.Uid, true, terminated}, map[string]any{"messageTags": sortedTags(metadataMessageTags(manifest.ApiVersion))})
	}
	return payloads
}

// clusterPayloads returns the payloads of the cluster. Its manifest holds a model of the nodes, with a hash of it as
// its version, which the cluster processor tests check.
func (env *Env) clusterPayloads() Payloads {
	payloads := Payloads{}
	payloads.add(payloadKey{clusterID, false, false}, map[string]any{
		"type": float64(orchestrator.K8sCluster), "uid": clusterID, "version": "v1", "contentType": "json",
		"envelope": "CollectorManifest", "messageTags": []string(nil),
	})
	payloads.add(payloadKey{clusterID, true, false}, map[string]any{"messageTags": []string(nil)})
	return payloads
}

// metadataMessageTags returns the tags of the message of a metadata model. Manifest messages have no tags of their own
// in the tests.
func metadataMessageTags(apiVersion string) []string {
	return []string{"kube_api_version:" + apiVersion}
}

// resourceNamed returns the resource of a collector.
func resourceNamed(t *testing.T, fullName string) Resource {
	for _, resource := range Resources() {
		for _, version := range resource.Versions {
			if resource.FixtureName(version) == fullName {
				return resource
			}
		}
	}
	require.Failf(t, "no resource", "no resource for %s", fullName)
	return Resource{}
}

var builtinFixtures = sync.OnceValue(resourceFixtures)

// hasMetadata reports whether the check sends a structured metadata model for the objects of a collector, as its
// fixture declares.
func hasMetadata(resource Resource, fullName string) bool {
	return !resource.IsCustomResource() && builtinFixtures()[fullName].MetadataType != nil
}

// manifestEnvelope returns the payload type of the manifests of a resource. Custom resources and CRDs have their own
// types.
func manifestEnvelope(resource Resource) string {
	switch {
	case resource.IsCustomResource():
		return "CollectorManifestCR"
	case resource.NodeType == orchestrator.K8sCRD:
		return "CollectorManifestCRD"
	}
	return "CollectorManifest"
}

// lastAppliedAnnotation is replaced in every manifest, as it holds the whole object as applied, secrets included.
const lastAppliedAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

// unifiedServiceTagKinds are the kinds whose manifests have the tags of the unified service tag labels.
var unifiedServiceTagKinds = []string{
	"ClusterRole", "ClusterRoleBinding", "CronJob", "DaemonSet", "Deployment", "HorizontalPodAutoscaler", "Ingress", "Job",
	"Namespace", "NetworkPolicy", "Node", "PersistentVolume", "PersistentVolumeClaim", "Pod", "PodDisruptionBudget",
	"ReplicaSet", "Role", "RoleBinding", "Service", "ServiceAccount", "StatefulSet", "VerticalPodAutoscaler",
}

// kindTags are the tags that the check derives from the status of an object of the kind, which the fixtures leave
// empty.
var kindTags = map[string][]string{
	"Node":             {"node_schedulable:true", "node_status:unknown"},
	"PersistentVolume": {"pv_phase:", "pv_type:<unknown>"},
}

// deletionTime replaces the deletion timestamp that the check sets when it sends a deletion, at the time of the run.
const deletionTime = "<deletion time>"

// baseManifest returns the manifest that the check sends for an object: the object, with the rules that the check
// applies to its kind. A test of a special case changes the base where the case differs.
func baseManifest(t *testing.T, resource Resource, fullName string, object runtime.Object, terminated bool) *model.Manifest {
	apiVersion := fullName[:strings.LastIndex(fullName, "/")]
	kind := resource.Kind
	if resource.IsCustomResource() {
		kind = object.GetObjectKind().GroupVersionKind().Kind
	}
	accessor, err := meta.Accessor(object)
	require.NoError(t, err)

	// The check marshals with jsoniter, which writes the zero creation timestamps that encoding/json omits.
	data, err := jsoniter.ConfigCompatibleWithStandardLibrary.Marshal(object)
	require.NoError(t, err)
	var content map[string]any
	require.NoError(t, json.Unmarshal(data, &content))
	content["apiVersion"], content["kind"] = apiVersion, kind
	metadata := content["metadata"].(map[string]any)
	if annotations, ok := metadata["annotations"].(map[string]any); ok && annotations[lastAppliedAnnotation] != nil {
		annotations[lastAppliedAnnotation] = "-"
	}
	if terminated {
		metadata["deletionTimestamp"] = deletionTime
	}
	scrubEnvValues(content)

	tags := []string{"kube_api_version:" + apiVersion}
	if env := accessor.GetLabels()["tags.datadoghq.com/env"]; env != "" && slices.Contains(unifiedServiceTagKinds, kind) {
		tags = append(tags, "env:"+env)
	}
	tags = append(tags, kindTags[kind]...)
	manifest := &model.Manifest{Type: int32(resource.NodeType), Uid: string(accessor.GetUID()), ResourceVersion: accessor.GetResourceVersion(),
		Version: "v1", ContentType: "json", Tags: tags, IsTerminated: terminated, Kind: kind, ApiVersion: apiVersion}
	switch kind {
	case "ConfigMap", "Secret":
		// The metadata informers of these kinds drop the managed fields, and the secret annotations.
		delete(metadata, "managedFields")
		if kind == "Secret" {
			delete(metadata, "annotations")
		}
	case "Node":
		manifest.NodeName = accessor.GetName()
	case "Pod":
		// The fixtures have no pod phase, and a deleted pod is terminating.
		manifest.Tags = append(manifest.Tags, map[bool]string{false: "pod_status:", true: "pod_status:terminating"}[terminated])
		if spec, ok := content["spec"].(map[string]any); ok && spec["nodeName"] != nil {
			manifest.NodeName = spec["nodeName"].(string)
		}
	}
	manifest.Content, err = json.Marshal(content)
	require.NoError(t, err)
	return manifest
}

// scrubEnvValues hides the values of the container variables, which the fixtures all name PASSWORD.
func scrubEnvValues(value any) {
	switch v := value.(type) {
	case map[string]any:
		for key, field := range v {
			if env, ok := field.([]any); ok && key == "env" {
				for _, variable := range env {
					variable.(map[string]any)["value"] = "********"
				}
				continue
			}
			scrubEnvValues(field)
		}
	case []any:
		for _, item := range v {
			scrubEnvValues(item)
		}
	}
}

// manifestValue returns a whole manifest as JSON values, with its content decoded so that a difference names the
// field, and with the values that the tests cannot predict replaced.
func manifestValue(t *testing.T, manifest *model.Manifest, envelope string, messageTags []string) map[string]any {
	value := payloadValue(t, Sent{UID: manifest.Uid, Model: manifest})
	switch {
	case manifest.Uid == clusterID:
		delete(value, "content")
		delete(value, "resourceVersion")
		delete(value, "tags")
	case manifest.Kind == "Pod":
		// The pod version is a hash of its metadata model, which the pod processor tests check.
		value["resourceVersion"] = "<hash>"
	}
	if content, ok := value["content"].(map[string]any); ok {
		if metadata, ok := content["metadata"].(map[string]any); ok && metadata["deletionTimestamp"] != nil {
			metadata["deletionTimestamp"] = deletionTime
		}
	}
	value["envelope"], value["messageTags"] = envelope, sortedTags(messageTags)
	return value
}

func sortedTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	return slices.Sorted(slices.Values(tags))
}

// payloadValue returns the whole model of a resource as JSON values, with the content of a manifest decoded, so that a
// difference names the field.
func payloadValue(t *testing.T, s Sent) map[string]any {
	value := jsonValue(t, s.Model)
	if manifest, ok := s.Model.(*model.Manifest); ok && len(manifest.Content) > 0 {
		var content any
		require.NoError(t, json.Unmarshal(manifest.Content, &content), "%s: manifest content", s.UID)
		value["content"] = content
	}
	sortUnorderedLists(value)
	return value
}

func jsonValue(t *testing.T, v any) map[string]any {
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

// assertNoSensitiveData checks that no payload carries a field that the check must remove: the fixtures put "secret"
// in the last-applied-configuration annotation and in a container environment variable, and set managed fields, which
// the manifest-only kinds remove.
func (env *Env) assertNoSensitiveData(t *testing.T, sent []Sent) {
	t.Helper()
	for _, s := range sent {
		assert.False(t, bytes.Contains(s.Content, []byte(`"secret"`)), "%s %s sends a sensitive value: %s", s.Envelope, s.UID, s.Content)
		if o := env.objectByUID(s.UID); o != nil && slices.Contains([]string{"v1/configmaps", "v1/secrets", "v1/helm-releases-configmaps"}, o.FullName) {
			assert.False(t, bytes.Contains(s.Content, []byte("managedFields")), "%s %s sends managed fields", s.Envelope, s.UID)
		}
	}
}
