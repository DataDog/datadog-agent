// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

// This file is a behavioral characterization (golden master) of the target
// matcher: which target a pod resolves to for every supported selector shape
// and for the "first match wins" ordering rule. It exercises the matcher
// exclusively through the public TargetMutator API (NewTargetMutator +
// getMatchingTarget), so the exact same suite runs against the legacy
// label-selector implementation and the policy-engine implementation and proves
// they agree.

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// matchCase is a single (pod -> matched target name) expectation.
type matchCase struct {
	name      string
	ns        string
	podLabels map[string]string
	want      string // matched target name, "" when no target matches
}

func newMatchTestWmeta(t *testing.T, namespaces ...workloadmeta.KubernetesMetadata) workloadmetamock.Mock {
	t.Helper()
	wmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Supply(coreconfig.Params{}),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() coreconfig.Component { return coreconfig.NewMock(t) }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	for i := range namespaces {
		wmeta.Set(&namespaces[i])
	}
	return wmeta
}

func newMatchMutator(t *testing.T, yamlCfg string, wmeta workloadmeta.Component) *TargetMutator {
	t.Helper()
	mockConfig := configmock.NewFromYAML(t, yamlCfg)
	mockConfig.SetInTest("admission_controller.auto_instrumentation.container_registry", "registry")
	config, err := NewConfig(mockConfig)
	require.NoError(t, err)
	m, err := NewTargetMutator(config, wmeta, imageResolver, nil)
	require.NoError(t, err)
	return m
}

// runMatchCases builds one mutator per case from the shared config so that the
// workloadmeta store can carry per-case namespace labels.
func runMatchCases(t *testing.T, yamlCfg string, cases []matchCase, namespaces ...workloadmeta.KubernetesMetadata) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wmeta := newMatchTestWmeta(t, namespaces...)
			m := newMatchMutator(t, yamlCfg, wmeta)
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: tc.ns, Labels: tc.podLabels}}
			got := ""
			if target := m.getMatchingTarget(pod); target != nil {
				got = target.name
			}
			require.Equal(t, tc.want, got)
		})
	}
}

// TestMatching_Precedence verifies the "first match wins" ordering rule when a
// pod satisfies more than one target.
func TestMatching_Precedence(t *testing.T) {
	const cfg = `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: "db"
        podSelector:
          matchLabels:
            app: "db"
        ddTraceVersions:
          java: "default"
      - name: "router"
        podSelector:
          matchLabels:
            webserver: "user"
        ddTraceVersions:
          php: "default"
      - name: "catch-all"
        ddTraceVersions:
          js: "default"
`
	runMatchCases(t, cfg, []matchCase{
		{name: "db pod hits db", podLabels: map[string]string{"app": "db"}, want: "db"},
		{name: "router pod hits router", podLabels: map[string]string{"webserver": "user"}, want: "router"},
		{name: "pod matching db and router resolves to the first", podLabels: map[string]string{"app": "db", "webserver": "user"}, want: "db"},
		{name: "pod matching router and catch-all resolves to router", podLabels: map[string]string{"webserver": "user", "x": "y"}, want: "router"},
		{name: "unrelated pod falls through to catch-all", podLabels: map[string]string{"other": "x"}, want: "catch-all"},
		{name: "empty pod falls through to catch-all", podLabels: map[string]string{}, want: "catch-all"},
	})
}

// TestMatching_PodMatchLabels verifies that pod matchLabels are ANDed and that
// extra labels on the pod do not prevent a match.
func TestMatching_PodMatchLabels(t *testing.T) {
	const cfg = `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: "multi"
        podSelector:
          matchLabels:
            app: "web"
            tier: "frontend"
        ddTraceVersions:
          java: "default"
`
	runMatchCases(t, cfg, []matchCase{
		{name: "all labels present matches", podLabels: map[string]string{"app": "web", "tier": "frontend"}, want: "multi"},
		{name: "extra labels still match", podLabels: map[string]string{"app": "web", "tier": "frontend", "extra": "x"}, want: "multi"},
		{name: "missing one label does not match", podLabels: map[string]string{"app": "web"}, want: ""},
		{name: "wrong value does not match", podLabels: map[string]string{"app": "web", "tier": "backend"}, want: ""},
	})
}

// TestMatching_PodExpressionIn covers the In operator on a pod selector.
func TestMatching_PodExpressionIn(t *testing.T) {
	const cfg = `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: "in"
        podSelector:
          matchExpressions:
            - key: "lang"
              operator: "In"
              values: ["java", "go"]
`
	runMatchCases(t, cfg, []matchCase{
		{name: "value in set matches", podLabels: map[string]string{"lang": "go"}, want: "in"},
		{name: "value not in set does not match", podLabels: map[string]string{"lang": "ruby"}, want: ""},
		{name: "absent key does not match", podLabels: map[string]string{}, want: ""},
	})
}

// TestMatching_PodExpressionNotIn covers the NotIn operator, including the
// Kubernetes rule that an absent key matches NotIn.
func TestMatching_PodExpressionNotIn(t *testing.T) {
	const cfg = `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: "notin"
        podSelector:
          matchExpressions:
            - key: "app"
              operator: "NotIn"
              values: ["app1", "app2"]
`
	runMatchCases(t, cfg, []matchCase{
		{name: "value outside set matches", podLabels: map[string]string{"app": "app3"}, want: "notin"},
		{name: "value in set does not match", podLabels: map[string]string{"app": "app1"}, want: ""},
		{name: "absent key matches notin", podLabels: map[string]string{}, want: "notin"},
	})
}

// TestMatching_PodExpressionExists covers Exists and DoesNotExist.
func TestMatching_PodExpressionExists(t *testing.T) {
	const existsCfg = `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: "exists"
        podSelector:
          matchExpressions:
            - key: "tier"
              operator: "Exists"
`
	runMatchCases(t, existsCfg, []matchCase{
		{name: "key present matches", podLabels: map[string]string{"tier": "frontend"}, want: "exists"},
		{name: "key absent does not match", podLabels: map[string]string{"other": "x"}, want: ""},
	})

	const doesNotExistCfg = `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: "dne"
        podSelector:
          matchExpressions:
            - key: "deprecated"
              operator: "DoesNotExist"
`
	runMatchCases(t, doesNotExistCfg, []matchCase{
		{name: "key absent matches", podLabels: map[string]string{"other": "x"}, want: "dne"},
		{name: "key present does not match", podLabels: map[string]string{"deprecated": "true"}, want: ""},
	})
}

// TestMatching_NamespaceMatchNames covers namespace selection by name. matchNames
// does not require namespace labels, so no workloadmeta entry is needed.
func TestMatching_NamespaceMatchNames(t *testing.T) {
	const cfg = `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: "by-name"
        namespaceSelector:
          matchNames: ["payments", "billing"]
        ddTraceVersions:
          java: "default"
`
	runMatchCases(t, cfg, []matchCase{
		{name: "listed namespace matches", ns: "billing", want: "by-name"},
		{name: "other listed namespace matches", ns: "payments", want: "by-name"},
		{name: "unlisted namespace does not match", ns: "default", want: ""},
	})
}

// TestMatching_NamespaceMatchLabels covers namespace selection by label, which
// requires the namespace metadata to be present in the workloadmeta store.
func TestMatching_NamespaceMatchLabels(t *testing.T) {
	const cfg = `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: "by-ns-label"
        namespaceSelector:
          matchLabels:
            instrument: "true"
        ddTraceVersions:
          java: "default"
`
	runMatchCases(t, cfg, []matchCase{
		{name: "labeled namespace matches", ns: "labeled", want: "by-ns-label"},
		{name: "unlabeled namespace does not match", ns: "plain", want: ""},
	},
		newTestNamespace("labeled", map[string]string{"instrument": "true"}),
		newTestNamespace("plain", map[string]string{"other": "x"}),
	)
}

// TestMatching_NamespaceExpressions covers namespace matchExpressions (In and
// Exists ANDed together).
func TestMatching_NamespaceExpressions(t *testing.T) {
	const cfg = `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: "ns-expr"
        namespaceSelector:
          matchExpressions:
            - key: "team"
              operator: "In"
              values: ["payments"]
            - key: "instrument"
              operator: "Exists"
        ddTraceVersions:
          java: "default"
`
	runMatchCases(t, cfg, []matchCase{
		{name: "both expressions satisfied matches", ns: "good", want: "ns-expr"},
		{name: "in fails does not match", ns: "wrong-team", want: ""},
		{name: "exists fails does not match", ns: "no-instrument", want: ""},
	},
		newTestNamespace("good", map[string]string{"team": "payments", "instrument": "yes"}),
		newTestNamespace("wrong-team", map[string]string{"team": "other", "instrument": "yes"}),
		newTestNamespace("no-instrument", map[string]string{"team": "payments"}),
	)
}

// TestMatching_CombinedNamespaceAndPod verifies that namespace and pod selectors
// on the same target are ANDed.
func TestMatching_CombinedNamespaceAndPod(t *testing.T) {
	const cfg = `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: "combined"
        namespaceSelector:
          matchNames: ["login"]
        podSelector:
          matchLabels:
            app: "resolver"
        ddTraceVersions:
          java: "default"
`
	runMatchCases(t, cfg, []matchCase{
		{name: "namespace and pod both match", ns: "login", podLabels: map[string]string{"app": "resolver"}, want: "combined"},
		{name: "pod mismatch in matching namespace", ns: "login", podLabels: map[string]string{"app": "other"}, want: ""},
		{name: "matching pod in wrong namespace", ns: "other", podLabels: map[string]string{"app": "resolver"}, want: ""},
	})
}

// TestMatching_EmptyTargetMatchesEverything verifies that a target without any
// selector matches every pod in every namespace.
func TestMatching_EmptyTargetMatchesEverything(t *testing.T) {
	const cfg = `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: "default"
        ddTraceVersions:
          java: "default"
`
	runMatchCases(t, cfg, []matchCase{
		{name: "labeled pod matches", ns: "whatever", podLabels: map[string]string{"any": "thing"}, want: "default"},
		{name: "empty pod matches", ns: "elsewhere", want: "default"},
	})
}

// TestMatching_DisabledNamespace verifies that a disabled namespace short-circuits
// matching even when a target would otherwise apply.
func TestMatching_DisabledNamespace(t *testing.T) {
	const cfg = `
apm_config:
  instrumentation:
    enabled: true
    disabled_namespaces: ["infra"]
    targets:
      - name: "all"
        ddTraceVersions:
          java: "default"
`
	runMatchCases(t, cfg, []matchCase{
		{name: "disabled namespace never matches", ns: "infra", want: ""},
		{name: "other namespace matches", ns: "app", want: "all"},
	})
}
