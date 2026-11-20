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
	"reflect"
	"strings"
	"testing"
)

func writeFixture(t *testing.T, filename, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateBundle(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "bundle")
	writeFixture(t, filepath.Join(source, "helper.go"), `package bundle
func helper() {}
func NewScaleWorkloadHandler() *ScaleWorkloadHandler { return nil }
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
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 6 {
		t.Fatalf("generated %d files, want 5 supported methods and entrypoint", len(files))
	}
	for filename, contents := range files {
		if _, err := parser.ParseFile(token.NewFileSet(), filename, contents, parser.AllErrors); err != nil {
			t.Fatal(err)
		}
	}
	create := string(files["create_horizontal_pod_autoscaler.gen.go"])
	for _, want := range []string{
		`v1 "k8s.io/api/autoscaling/v2"`,
		`metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"`,
		"AutoscalingV2().HorizontalPodAutoscalers(inputs.Namespace).Create(",
		"*v1.HorizontalPodAutoscalerStatus", "map[string][]v1.LocalType",
		`errors.New("namespace is required")`, `errors.New("body is required")`,
	} {
		if !strings.Contains(create, want) {
			t.Errorf("Create handler missing %q", want)
		}
	}
	if strings.Contains(create, "TypeMeta") {
		t.Error("TypeMeta must not be exposed in outputs")
	}
	list := string(files["list_horizontal_pod_autoscaler.gen.go"])
	if strings.Contains(list, `namespace is required`) || !strings.Contains(list, "Items:") || !strings.Contains(list, "ListMeta:") {
		t.Error("List must allow all namespaces and return pagination metadata")
	}
	patch := string(files["patch_horizontal_pod_autoscaler.gen.go"])
	for _, want := range []string{"inputs.PatchFields == nil", "json.Marshal(inputs.Body)", "typesv1.JSONPatchType"} {
		if !strings.Contains(patch, want) {
			t.Errorf("Patch handler missing %q", want)
		}
	}
	entry := string(files["entrypoint.gen.go"])
	if !strings.Contains(entry, `"scaleWorkload": NewScaleWorkloadHandler()`) || strings.Contains(entry, "NewHelperHandler") {
		t.Error("entrypoint must register only handwritten action constructors")
	}
	cfg.Resources = []string{"MissingResource"}
	if _, err := generateBundle(root, "autoscaling", source, cfg); err == nil {
		t.Error("missing configured resource must fail generation")
	}
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
			if !ok || a.Name != tc.actionName || snake(a.Name) != tc.filename {
				t.Fatalf("unexpected action: %+v, filename %s", a, snake(a.Name))
			}
			contents, err := render("actions", a)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(contents), "inputs.Namespace") || strings.Contains(string(contents), `"errors"`) {
				t.Error("cluster-scoped collection deletion must not require namespace or unused imports")
			}
			get, ok := methodAction("Get", "networking", cfg, res)
			if !ok || get.Name != "get"+tc.name {
				t.Errorf("resource singular changed: %+v", get)
			}
		})
	}
}

func TestQualifyType(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"map[string][]*Thing", "map[string][]*v1.Thing"},
		{"[3]byte", "[3]byte"},
		{"metav1.ObjectMeta", "metav1.ObjectMeta"},
		{"float64", "float64"},
	} {
		expr, err := parser.ParseExpr(tc.input)
		if err != nil {
			t.Fatal(err)
		}
		got, _, err := qualifyType(expr, "k8s.io/api/core/v1", map[string]string{"metav1": "k8s.io/apimachinery/pkg/apis/meta/v1"})
		if err != nil || got != tc.want {
			t.Errorf("qualifyType(%s) = %q, %v, want %q", tc.input, got, err, tc.want)
		}
	}
	expr, _ := parser.ParseExpr("unknown.Type")
	if _, _, err := qualifyType(expr, "", nil); err == nil {
		t.Error("unresolved imports must fail rather than produce invalid code")
	}
}

func TestManualActions(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, filepath.Join(dir, "support.go"), `package bundle
func helper() {}
func NewZuluHandler() *ZuluHandler { return nil }
func NewAlphaHandler() *AlphaHandler { return nil }
func (*Thing) NewReceiverHandler() {}
`)
	got, err := manualActions(dir)
	if err != nil || !reflect.DeepEqual(got, []string{"alpha", "zulu"}) {
		t.Fatalf("manualActions = %v, %v", got, err)
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
	if err := run(root, "discovery", source, out, true); err == nil {
		t.Fatal("check must detect missing files")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("check must not create output directory")
	}
	if err := run(root, "discovery", source, out, false); err != nil {
		t.Fatal(err)
	}
	if err := run(root, "discovery", source, out, true); err != nil {
		t.Fatalf("fresh output failed check: %v", err)
	}
	filename := filepath.Join(out, "discovery/get_endpoint_slice.gen.go")
	before, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filename, "stale content")
	obsolete := filepath.Join(out, "discovery/obsolete.gen.go")
	writeFixture(t, obsolete, "old action")
	handwritten := filepath.Join(out, "discovery/manual.go")
	writeFixture(t, handwritten, "handwritten")
	unselected := filepath.Join(out, "core/keep.gen.go")
	writeFixture(t, unselected, "unselected bundle")
	err = run(root, "discovery", source, out, true)
	if err == nil || !strings.Contains(err.Error(), "obsolete.gen.go") || !strings.Contains(err.Error(), "get_endpoint_slice.gen.go") {
		t.Fatalf("check must report changed and obsolete files: %v", err)
	}
	if got, _ := os.ReadFile(filename); string(got) != "stale content" {
		t.Fatal("check modified a file")
	}
	if _, err := os.Stat(obsolete); err != nil {
		t.Fatal("check removed a file")
	}
	if err := run(root, "discovery", source, out, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filename); string(got) != string(before) {
		t.Fatal("regeneration did not restore deterministic output")
	}
	if _, err := os.Stat(obsolete); !os.IsNotExist(err) {
		t.Fatal("regeneration must remove obsolete generated files")
	}
	for _, keep := range []string{handwritten, unselected} {
		if _, err := os.Stat(keep); err != nil {
			t.Fatal("regeneration removed a handwritten file or an unselected bundle")
		}
	}
	if err := run(root, "unknown", source, out, false); err == nil {
		t.Fatal("unknown bundle must fail")
	}
}
