// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"bytes"
	"encoding/json"
	"testing"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
)

func TestMutatePreservesListEntries(t *testing.T) {
	const raw = `{"spec":{"containers":[{"name":"app","image":"app:v1"}],"volumes":[{"name":"data","emptyDir":{},"futureVolumeField":"data"},{"name":"cache","emptyDir":{"medium":"Memory"},"futureVolumeField":"cache"}],"tolerations":[{"key":"k","operator":"Equal","value":"a","effect":"NoExecute","tolerationSeconds":10,"futureField":"a","futureNumber":9007199254740993},{"key":"k","operator":"Equal","value":"b","effect":"NoExecute","futureField":"b"}]}}`
	type object = map[string]any
	list := func(spec object, field string) []any { return spec[field].([]any) }
	entry := func(spec object, field string, i int) object { return list(spec, field)[i].(object) }
	tests := []struct {
		name   string
		mutate func(*corev1.Pod)
		want   func(object)
	}{
		{"append toleration", func(p *corev1.Pod) {
			p.Spec.Tolerations = append(p.Spec.Tolerations, corev1.Toleration{Key: "spot", Operator: corev1.TolerationOpExists})
		}, func(s object) {
			s["tolerations"] = append(list(s, "tolerations"), object{"key": "spot", "operator": "Exists"})
		}},
		{"remove first duplicate selector", func(p *corev1.Pod) {
			p.Spec.Tolerations = p.Spec.Tolerations[1:]
		}, func(s object) { s["tolerations"] = list(s, "tolerations")[1:] }},
		{"reorder tolerations", func(p *corev1.Pod) {
			p.Spec.Tolerations[0], p.Spec.Tolerations[1] = p.Spec.Tolerations[1], p.Spec.Tolerations[0]
		}, func(s object) {
			items := list(s, "tolerations")
			items[0], items[1] = items[1], items[0]
		}},
		{"edit duplicate while reserving unchanged occurrence", func(p *corev1.Pod) {
			p.Spec.Tolerations[0], p.Spec.Tolerations[1] = p.Spec.Tolerations[1], p.Spec.Tolerations[0]
			p.Spec.Tolerations[0].Value = "updated"
		}, func(s object) {
			items := list(s, "tolerations")
			items[0], items[1] = items[1], items[0]
			entry(s, "tolerations", 0)["value"] = "updated"
		}},
		{"remove known toleration field", func(p *corev1.Pod) {
			p.Spec.Tolerations[0].TolerationSeconds = nil
		}, func(s object) { delete(entry(s, "tolerations", 0), "tolerationSeconds") }},
		{"edit large known integer", func(p *corev1.Pod) {
			seconds := int64(9007199254740993)
			p.Spec.Tolerations[0].TolerationSeconds = &seconds
		}, func(s object) { entry(s, "tolerations", 0)["tolerationSeconds"] = json.Number("9007199254740993") }},
		{"replace toleration selector", func(p *corev1.Pod) {
			p.Spec.Tolerations = []corev1.Toleration{{Key: "new", Operator: corev1.TolerationOpExists}}
		}, func(s object) { s["tolerations"] = []any{object{"key": "new", "operator": "Exists"}} }},
		{"remove all tolerations", func(p *corev1.Pod) { p.Spec.Tolerations = nil }, func(s object) { delete(s, "tolerations") }},
		{"edit volume", func(p *corev1.Pod) {
			p.Spec.Volumes[0].EmptyDir.Medium = corev1.StorageMediumMemory
		}, func(s object) { entry(s, "volumes", 0)["emptyDir"].(object)["medium"] = "Memory" }},
		{"replace volume source", func(p *corev1.Pod) {
			p.Spec.Volumes[0].VolumeSource = corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "credentials"}}
		}, func(s object) {
			v := entry(s, "volumes", 0)
			delete(v, "emptyDir")
			v["secret"] = object{"secretName": "credentials"}
		}},
		{"remove known volume field", func(p *corev1.Pod) {
			p.Spec.Volumes[1].EmptyDir.Medium = ""
		}, func(s object) { delete(entry(s, "volumes", 1)["emptyDir"].(object), "medium") }},
		{"reorder and edit volumes", func(p *corev1.Pod) {
			p.Spec.Volumes[0], p.Spec.Volumes[1] = p.Spec.Volumes[1], p.Spec.Volumes[0]
			p.Spec.Volumes[1].EmptyDir.Medium = corev1.StorageMediumMemory
		}, func(s object) {
			items := list(s, "volumes")
			items[0], items[1] = items[1], items[0]
			entry(s, "volumes", 1)["emptyDir"].(object)["medium"] = "Memory"
		}},
		{"remove volume", func(p *corev1.Pod) { p.Spec.Volumes = p.Spec.Volumes[1:] }, func(s object) { s["volumes"] = list(s, "volumes")[1:] }},
		{"append volume", func(p *corev1.Pod) {
			p.Spec.Volumes = append(p.Spec.Volumes, corev1.Volume{Name: "new", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}})
		}, func(s object) { s["volumes"] = append(list(s, "volumes"), object{"name": "new", "emptyDir": object{}}) }},
		{"remove all volumes", func(p *corev1.Pod) { p.Spec.Volumes = nil }, func(s object) { delete(s, "volumes") }},
	}
	decode := func(t *testing.T, data []byte) object {
		t.Helper()
		var value object
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		require.NoError(t, decoder.Decode(&value))
		return value
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patchBytes, err := Mutate([]byte(raw), "default", "test", func(p *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
				tt.mutate(p)
				return true, nil
			}, nil)
			require.NoError(t, err)
			patch, err := jsonpatch.DecodePatch(patchBytes)
			require.NoError(t, err)
			actual, err := patch.Apply([]byte(raw))
			require.NoError(t, err)
			expected := decode(t, []byte(raw))
			tt.want(expected["spec"].(object))
			require.Equal(t, expected, decode(t, actual))
		})
	}
}

func TestMutateNormalizesThenEditsVolume(t *testing.T) {
	raw := []byte(`{"spec":{"containers":[{"name":"app","image":"app:v1"}],"volumes":[{"name":"v","emptyDir":{},"futureField":"first"},{"name":"v","emptyDir":{},"futureField":"second"}]}}`)
	patchBytes, err := Mutate(raw, "default", "test", func(p *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
		p.Spec.Volumes[0].EmptyDir.Medium = corev1.StorageMediumMemory
		return true, nil
	}, nil)
	require.NoError(t, err)
	patch, err := jsonpatch.DecodePatch(patchBytes)
	require.NoError(t, err)
	actual, err := patch.Apply(raw)
	require.NoError(t, err)
	require.JSONEq(t, `{"spec":{"containers":[{"name":"app","image":"app:v1"}],"volumes":[{"name":"v","emptyDir":{"medium":"Memory"},"futureField":"first"}]}}`, string(actual))
}
