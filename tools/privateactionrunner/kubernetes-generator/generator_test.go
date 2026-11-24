// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeFixture(t *testing.T, filename, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
	require.NoError(t, os.WriteFile(filename, []byte(content), 0o644))
}

func TestGenerateBundle(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "bundle")
	writeFixture(t, filepath.Join(source, "helper.go"), `package bundle
func helper() {}
func NewZuluHandler() *ZuluHandler { return nil }
func NewAlphaHandler() *AlphaHandler { return nil }
func (*Thing) NewReceiverHandler() {}
`)
	writeFixture(t, filepath.Join(source, "ignored_test.go"), "not Go syntax")
	writeFixture(t, filepath.Join(source, "old.gen.go"), "not Go syntax")
	writeFixture(t, filepath.Join(root, "k8s.io/api/autoscaling/v2/types.go"), `package v2
import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
type HorizontalPodAutoscaler struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Spec HorizontalPodAutoscalerSpec
	Status *HorizontalPodAutoscalerStatus
	Values map[string][]LocalType
}
`)
	writeFixture(t, filepath.Join(root, "k8s.io/client-go/kubernetes/typed/autoscaling/v2/hpa.go"), `package v2
type HorizontalPodAutoscalersGetter interface {
	HorizontalPodAutoscalers(namespace string) HorizontalPodAutoscalerInterface
}
type HorizontalPodAutoscalerInterface interface {
	Create()
	Get()
	List()
	Patch()
	DeleteCollection()
	Watch()
	Apply()
	UpdateStatus()
	HorizontalPodAutoscalerExpansion
}
`)
	cfg := targetConfig{Version: "v2", CopyrightYear: 2026, Resources: []string{"HorizontalPodAutoscaler"}}
	files, err := generateBundle(root, "autoscaling", source, cfg)
	require.NoError(t, err)
	require.Len(t, files, 6, "five supported methods and entrypoint")
	for filename, contents := range files {
		_, err := parser.ParseFile(token.NewFileSet(), filename, contents, parser.AllErrors)
		require.NoError(t, err)
	}

	create := string(files["create_horizontal_pod_autoscaler.gen.go"])
	for _, want := range []string{
		`v1 "k8s.io/api/autoscaling/v2"`,
		`metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"`,
		"AutoscalingV2().HorizontalPodAutoscalers(inputs.Namespace).Create(",
		"*v1.HorizontalPodAutoscalerStatus", "map[string][]v1.LocalType",
		`errors.New("namespace is required")`, `errors.New("body is required")`,
	} {
		require.Contains(t, create, want)
	}
	require.NotContains(t, create, "TypeMeta")

	list := string(files["list_horizontal_pod_autoscaler.gen.go"])
	require.NotContains(t, list, "namespace is required", "List must allow all namespaces")
	require.Contains(t, list, "Items:")
	require.Contains(t, list, "ListMeta:")
	patch := string(files["patch_horizontal_pod_autoscaler.gen.go"])
	for _, want := range []string{"inputs.PatchFields == nil", "json.Marshal(inputs.Body)", "typesv1.JSONPatchType"} {
		require.Contains(t, patch, want)
	}

	manual, err := manualActions(source)
	require.NoError(t, err)
	require.Equal(t, []string{"alpha", "zulu"}, manual)
	entry := string(files["entrypoint.gen.go"])
	require.Contains(t, entry, `"alpha": NewAlphaHandler()`)
	require.Contains(t, entry, `"zulu":  NewZuluHandler()`)
	require.NotContains(t, entry, "NewHelperHandler")
	require.NotContains(t, entry, "NewReceiverHandler")
	cfg.Resources = []string{"MissingResource"}
	_, err = generateBundle(root, "autoscaling", source, cfg)
	require.ErrorContains(t, err, "no client interface for resource MissingResource")
}

func TestMethodContracts(t *testing.T) {
	cfg := targetConfig{Version: "v1", CopyrightYear: 2026}
	for _, tc := range []struct {
		name, plural, actionName, filename string
	}{
		{"Endpoints", "Endpoints", "deleteMultipleEndpoints", "delete_multiple_endpoints"},
		{"Ingress", "Ingresses", "deleteMultipleIngresses", "delete_multiple_ingresses"},
		{"IPAddress", "IPAddresses", "deleteMultipleIPAddresses", "delete_multiple_ip_addresses"},
		{"ServiceCIDR", "ServiceCIDRs", "deleteMultipleServiceCIDRS", "delete_multiple_service_cidrs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := resource{Name: tc.name, NamePluralized: tc.plural}
			a, ok := methodAction("DeleteCollection", "networking", cfg, res)
			require.True(t, ok)
			require.Equal(t, tc.actionName, a.Name)
			require.Equal(t, tc.filename, snake(a.Name))
			contents, err := render("actions", a)
			require.NoError(t, err)
			require.NotContains(t, string(contents), "inputs.Namespace")
			require.NotContains(t, string(contents), `"errors"`)
			get, ok := methodAction("Get", "networking", cfg, res)
			require.True(t, ok)
			require.Equal(t, "get"+tc.name, get.Name)
		})
	}
}

func TestRunCheckAndCleanup(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	out := filepath.Join(root, "out")
	writeFixture(t, filepath.Join(source, "discovery/helper.go"), "package discovery\n")
	writeFixture(t, filepath.Join(root, "k8s.io/api/discovery/v1/types.go"), `package v1
type EndpointSlice struct { AddressType string }
`)
	writeFixture(t, filepath.Join(root, "k8s.io/client-go/kubernetes/typed/discovery/v1/slice.go"), `package v1
type EndpointSlicesGetter interface { EndpointSlices(namespace string) EndpointSliceInterface }
type EndpointSliceInterface interface { Get() }
`)
	require.ErrorContains(t, run(root, "discovery", source, out, true), "stale generated files")
	require.NoDirExists(t, out, "check must not create output directory")
	require.NoError(t, run(root, "discovery", source, out, false))
	require.NoError(t, run(root, "discovery", source, out, true))
	filename := filepath.Join(out, "discovery/get_endpoint_slice.gen.go")
	before, err := os.ReadFile(filename)
	require.NoError(t, err)
	writeFixture(t, filename, "stale content")
	obsolete := filepath.Join(out, "discovery/obsolete.gen.go")
	writeFixture(t, obsolete, "old action")
	handwritten := filepath.Join(out, "discovery/manual.go")
	writeFixture(t, handwritten, "handwritten")
	unselected := filepath.Join(out, "core/keep.gen.go")
	writeFixture(t, unselected, "unselected bundle")

	err = run(root, "discovery", source, out, true)
	require.ErrorContains(t, err, "obsolete.gen.go")
	require.ErrorContains(t, err, "get_endpoint_slice.gen.go")
	got, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, "stale content", string(got), "check must not modify files")
	require.FileExists(t, obsolete, "check must not remove files")

	require.NoError(t, run(root, "discovery", source, out, false))
	got, err = os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, before, got, "regeneration must restore deterministic output")
	require.NoFileExists(t, obsolete)
	require.FileExists(t, handwritten)
	require.FileExists(t, unselected)
	require.ErrorContains(t, run(root, "unknown", source, out, false), "unknown bundle")
}
