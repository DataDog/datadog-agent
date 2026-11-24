// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package suite

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	vpav1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	kscheme "k8s.io/client-go/kubernetes/scheme"

	orchestratorcheck "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator"
)

// Fixture is an object of the cluster, with what the fake cluster needs to serve it.
type Fixture struct {
	// Collector is the full name of the collector of the object, like apps/v1/deployments.
	Collector string
	Object    runtime.Object
	// Discovery is what API discovery reports for the resource of the object. It is nil for the collectors that read
	// the objects of another resource.
	Discovery *metav1.APIResource

	groupVersion string
	gvr          schema.GroupVersionResource
}

// pseudoCollectors read the objects of another resource, which API discovery reports.
var pseudoCollectors = []string{"v1/helm-releases-configmaps", "v1/terminated-pods"}

// endpointSlices are listed through the dynamic client.
var endpointSlices = schema.GroupVersionResource{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"}

// builtinFixtures returns a fixture for each collector of a built-in resource, with its own copy of the object.
func builtinFixtures(t testing.TB) []Fixture {
	t.Helper()
	fixtures, err := decodedBuiltinFixtures()
	require.NoError(t, err)
	copies := slices.Clone(fixtures)
	for i := range copies {
		copies[i].Object = copies[i].Object.DeepCopyObject()
	}
	return copies
}

// decodedBuiltinFixtures decodes the manifests of the test data files once for the whole test binary. A section with a
// manifest is the fixture of the collector of its UID.
var decodedBuiltinFixtures = sync.OnceValues(func() ([]Fixture, error) {
	paths, err := filepath.Glob(filepath.Join(TestDataDir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	var fixtures []Fixture
	for _, path := range paths {
		sections, err := readSections(path)
		if err != nil {
			return nil, err
		}
		for _, uid := range slices.Sorted(maps.Keys(sections)) {
			manifest, found := sections[uid]["manifest"]
			if !found {
				continue
			}
			object, err := decodeFixture(manifest)
			if err != nil {
				return nil, fmt.Errorf("%s: %s: %w", path, uid, err)
			}
			f, err := newBuiltinFixture(strings.ReplaceAll(uid, "_", "/"), object)
			if err != nil {
				return nil, err
			}
			fixtures = append(fixtures, f)
		}
	}
	return fixtures, nil
})

func newBuiltinFixture(collector string, object runtime.Object) (Fixture, error) {
	gvk := object.GetObjectKind().GroupVersionKind()
	resource := collector[strings.LastIndex(collector, "/")+1:]
	f := Fixture{Collector: collector, groupVersion: gvk.GroupVersion().String(), gvr: gvk.GroupVersion().WithResource(resource)}
	if !slices.Contains(pseudoCollectors, collector) {
		accessor, err := meta.Accessor(object)
		if err != nil {
			return Fixture{}, err
		}
		// A cluster-scoped object has no namespace, like its manifest.
		f.Discovery = &metav1.APIResource{Name: resource, Kind: gvk.Kind, Namespaced: accessor.GetNamespace() != "", Verbs: []string{"list", "watch"}}
	}

	switch {
	case f.gvr == endpointSlices:
		content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
		if err != nil {
			return Fixture{}, err
		}
		u := &unstructured.Unstructured{Object: content}
		u.SetGroupVersionKind(gvk)
		f.Object = u
	default:
		// The informers of typed clients return objects without their type.
		object.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{})
		f.Object = object
	}
	return f, nil
}

// fixtureScheme holds the types of the built-in resources, to decode their manifests and find their resources.
type fixtureScheme struct {
	scheme *runtime.Scheme
	codecs serializer.CodecFactory
}

var fixtureCodecs = sync.OnceValue(func() fixtureScheme {
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{kscheme.AddToScheme, apiextensionsv1.AddToScheme, vpav1.AddToScheme} {
		if err := add(scheme); err != nil {
			panic(err)
		}
	}
	return fixtureScheme{scheme: scheme, codecs: serializer.NewCodecFactory(scheme)}
})

func decodeFixture(manifest []byte) (runtime.Object, error) {
	object, _, err := fixtureCodecs().codecs.UniversalDeserializer().Decode(manifest, nil, nil)
	return object, err
}

// sampleResource is the resource that the fixture of a group-wide custom resource serves.
const sampleResource = "samples"

// CustomResource returns the generated object of a built-in custom resource, by collector full name, like
// argoproj.io/v1alpha1/rollouts. A group-wide custom resource has the resource name samples.
func CustomResource(t testing.TB, collector string) runtime.Object {
	for _, f := range customResourceFixtures() {
		if f.Collector == collector {
			return f.Object.DeepCopyObject()
		}
	}
	require.Failf(t, "no custom resource fixture", "%s is not a built-in custom resource", collector)
	return nil
}

// isCustomResource reports whether the fixture is the object of a custom resource, built-in or not.
func (f Fixture) isCustomResource() bool {
	return slices.ContainsFunc(append(customResourceFixtures(), userCustomResourceFixtures()...), func(cr Fixture) bool {
		return cr.Collector == f.Collector
	})
}

// userCustomResourceFixtures are custom resources that the check does not know, which a check configuration can
// collect with crd_collectors.
var userCustomResourceFixtures = sync.OnceValue(func() []Fixture {
	gv := schema.GroupVersion{Group: "stable.example.com", Version: "v1"}
	return []Fixture{{
		Collector:    gv.String() + "/crontabs",
		Object:       &unstructured.Unstructured{Object: map[string]any{"apiVersion": gv.String(), "kind": "CronTab"}},
		Discovery:    &metav1.APIResource{Name: "crontabs", Kind: "CronTab", Namespaced: true, Verbs: []string{"list", "watch"}},
		groupVersion: gv.String(),
		gvr:          gv.WithResource("crontabs"),
	}}
})

// CronTab returns a custom resource that the check does not know: a stable.example.com/v1 CronTab, the example of the
// Kubernetes documentation.
func CronTab(name string) runtime.Object {
	u := userCustomResourceFixtures()[0].Object.DeepCopyObject().(*unstructured.Unstructured)
	u.SetName(name)
	u.SetNamespace("default")
	u.SetUID(types.UID(name))
	u.SetResourceVersion("1")
	u.Object["spec"] = map[string]any{"cronSpec": "* * * * */5"}
	return u
}

// CRD returns the CRD of the resource of a custom resource object. The API server reports the resource once its CRD
// is established.
func CRD(t *testing.T, object runtime.Object, established bool) *apiextensionsv1.CustomResourceDefinition {
	f := fixtureOf(object)
	require.NotNil(t, f.Discovery, "%T %s is not a known custom resource", object, uidOf(object))
	crd := crdOf(f)
	if !established {
		crd.Status.Conditions = nil
	}
	return crd
}

var customResourceFixtures = sync.OnceValue(func() []Fixture {
	var fixtures []Fixture
	for _, name := range orchestratorcheck.BuiltinCustomResourcesForTest() {
		parts := strings.Split(name, "/")
		group, version, resource := parts[0], parts[1], parts[2]
		if resource == "" {
			resource = sampleResource
		}
		kind := kindOf(resource)
		gv := schema.GroupVersion{Group: group, Version: version}
		collector := gv.String() + "/" + resource
		name, uid := objectName(collector), objectUID(collector)
		fixtures = append(fixtures, Fixture{
			Collector: collector,
			Object: &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": gv.String(),
				"kind":       kind,
				"metadata": map[string]any{
					"name": name, "namespace": "default", "uid": uid, "resourceVersion": "1",
					"annotations": map[string]any{"kubectl.kubernetes.io/last-applied-configuration": "secret"},
				},
				"spec": map[string]any{"replicas": int64(1)},
			}},
			Discovery:    &metav1.APIResource{Name: resource, Kind: kind, Namespaced: true, Verbs: []string{"list", "watch"}},
			groupVersion: gv.String(),
			gvr:          gv.WithResource(resource),
		})
	}
	return fixtures
})

// objectUID returns the UID of the fixture of a collector: the name of its files, like apps_v1_deployments.
func objectUID(collector string) string {
	return strings.ReplaceAll(collector, "/", "_")
}

// objectName returns the name of the fixture of a collector, which Kubernetes names allow, like apps-v1-deployments.
func objectName(collector string) string {
	return strings.NewReplacer("/", "-", ".", "-").Replace(collector)
}

// kindOf returns the kind of a generated custom resource: the singular of its resource name.
func kindOf(resource string) string {
	singular := strings.TrimSuffix(resource, "s")
	if base, found := strings.CutSuffix(resource, "ies"); found {
		singular = base + "y"
	} else if base, found := strings.CutSuffix(resource, "sses"); found {
		singular = base + "ss"
	}
	return strings.ToUpper(singular[:1]) + singular[1:]
}

// resourceOf returns the resource of an unstructured kind. The dynamic client guesses it wrongly for kinds such as
// Gateway.
func resourceOf(gvk schema.GroupVersionKind) schema.GroupVersionResource {
	for _, f := range append(customResourceFixtures(), userCustomResourceFixtures()...) {
		if f.gvr.GroupVersion() == gvk.GroupVersion() && f.Discovery.Kind == gvk.Kind {
			return f.gvr
		}
	}
	gvr, _ := meta.UnsafeGuessKindToResource(gvk)
	return gvr
}

// crdOf returns the established CRD of the resource of a custom resource fixture.
func crdOf(f Fixture) *apiextensionsv1.CustomResourceDefinition {
	group, version, _ := strings.Cut(f.groupVersion, "/")
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: f.Discovery.Name + "." + group, UID: types.UID(objectUID(f.Collector) + "_crd"), ResourceVersion: "1"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group:    group,
			Names:    apiextensionsv1.CustomResourceDefinitionNames{Plural: f.Discovery.Name, Kind: f.Discovery.Kind},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{Name: version, Served: true, Storage: true}},
		},
		Status: apiextensionsv1.CustomResourceDefinitionStatus{Conditions: []apiextensionsv1.CustomResourceDefinitionCondition{
			{Type: apiextensionsv1.Established, Status: apiextensionsv1.ConditionTrue},
		}},
	}
}

// withoutFixture are the collectors that have no fixture, with the reason.
var withoutFixture = map[string]string{
	"clusters":           "the cluster manifest combines the nodes",
	"v1/terminated-pods": "the collector reports the deletions of the pods of the v1/pods fixture",
}

// AssertFixturesCoverEveryCollector checks that every collector of a built-in resource has a fixture, so that the
// default environment has an object of each resource. The custom resource fixtures are generated from the list of the
// check.
func AssertFixturesCoverEveryCollector(t *testing.T) {
	env := Setup(t, WithObjects())
	var collectors []string
	for _, m := range orchestratorcheck.CollectorMetadataForTest(env.Config, env.Workloadmeta, env.Tagger) {
		if _, skipped := withoutFixture[m.FullName()]; !skipped && !slices.Contains(collectors, m.FullName()) {
			collectors = append(collectors, m.FullName())
		}
	}
	var fixtures []string
	for _, f := range builtinFixtures(t) {
		fixtures = append(fixtures, f.Collector)
	}
	assert.ElementsMatch(t, collectors, fixtures, "add a section with a manifest to testdata/<resource>.yaml for each new collector, or a reason to withoutFixture")
}
