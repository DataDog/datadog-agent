// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

// This file is a behavioral characterization of the target matcher: which
// target a pod resolves to for every supported selector shape, configuration
// first-wins (targets reversed at construction so the last-TRUE-wins matcher
// preserves config order), and static vs remote-config source precedence. It
// exercises matching through NewTargetMutator + getMatchingTarget.

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
	"github.com/DataDog/dd-policy-engine/go/policies"
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
	m, err := NewTargetMutator(config, wmeta, imageResolver, nil, nil)
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

// TestMatching_Precedence verifies configuration first-wins when a pod satisfies
// more than one target.
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

// TestMatching_EvaluationSources pins which source wins: static targets
// (including enabledNamespaces), remote-config policies (wire last-TRUE-wins),
// then the SSI inject-all default when both are absent.
//
//	SSI | static targets | RC          | Decision
//	off | —              | none        | nothing
//	off | —              | policies    | last matching policy, else nothing
//	on  | none           | none        | everything
//	on  | none           | policies    | last matching policy, else nothing
//	on  | present        | none        | first matching target, else nothing
//	on  | present        | policies    | last matching policy, else first matching target, else nothing
func TestMatching_EvaluationSources(t *testing.T) {
	const ssiOff = `
apm_config:
  instrumentation:
    enabled: false
`
	const ssiOnNoTargets = `
apm_config:
  instrumentation:
    enabled: true
`
	const ssiOnEnabledNamespaces = `
apm_config:
  instrumentation:
    enabled: true
    enabled_namespaces:
      - app-ns
`
	const ssiOnTargets = `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: "helm-python"
        podSelector:
          matchLabels:
            language: "python"
        ddTraceVersions:
          python: "default"
      - name: "helm-python-also"
        podSelector:
          matchLabels:
            language: "python"
        ddTraceVersions:
          python: "v1"
`

	rcPolicies := []policies.Policy{
		{
			Name:    "rc-default",
			Rules:   policies.AlwaysTrue(),
			Outcome: policies.Outcome{Inject: true, InjectSet: true, TracerVersions: map[string]string{"java": "default"}},
		},
		podLabelPolicy("rc-db", "app", "db", true, map[string]string{"java": "v1"}),
		podLabelPolicy("rc-legacy-deny", "app", "legacy", false, nil),
	}

	type want struct {
		name       string
		fromPolicy bool
	}
	nothing := want{}
	helm := func(name string) want { return want{name: name} }
	rc := func(name string) want { return want{name: name, fromPolicy: true} }

	assertMatch := func(t *testing.T, m *TargetMutator, ns string, labels map[string]string, w want) {
		t.Helper()
		name, fromPolicy := matchedTarget(t, m, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Labels: labels}})
		require.Equal(t, w.name, name)
		require.Equal(t, w.fromPolicy, fromPolicy)
	}

	t.Run("ssi off / no RC / nothing", func(t *testing.T) {
		m := newMatchMutator(t, ssiOff, newMatchTestWmeta(t))
		assertMatch(t, m, "ns", map[string]string{"app": "db"}, nothing)
		assertMatch(t, m, "ns", map[string]string{"app": "other"}, nothing)
	})

	t.Run("ssi off / RC / last matching policy, else nothing", func(t *testing.T) {
		m := newMatchMutator(t, ssiOff, newMatchTestWmeta(t))
		require.NoError(t, m.SetRemotePolicies(rcPolicies))
		assertMatch(t, m, "ns", map[string]string{"app": "legacy"}, nothing)
		assertMatch(t, m, "ns", map[string]string{"app": "db"}, rc("rc-db"))
		assertMatch(t, m, "ns", map[string]string{"app": "other"}, rc("rc-default"))
	})

	t.Run("ssi on / no targets / no RC / everything", func(t *testing.T) {
		m := newMatchMutator(t, ssiOnNoTargets, newMatchTestWmeta(t))
		assertMatch(t, m, "ns", map[string]string{"app": "db"}, helm("default"))
		assertMatch(t, m, "ns", map[string]string{"app": "other"}, helm("default"))
	})

	t.Run("ssi on / no targets / RC / last matching policy, else nothing", func(t *testing.T) {
		m := newMatchMutator(t, ssiOnNoTargets, newMatchTestWmeta(t))
		require.NoError(t, m.SetRemotePolicies(rcPolicies))
		assertMatch(t, m, "ns", map[string]string{"app": "legacy"}, nothing)
		assertMatch(t, m, "ns", map[string]string{"app": "db"}, rc("rc-db"))
		assertMatch(t, m, "ns", map[string]string{"app": "other"}, rc("rc-default"))
	})

	t.Run("ssi on / enabledNamespaces / no RC / first matching target, else nothing", func(t *testing.T) {
		m := newMatchMutator(t, ssiOnEnabledNamespaces, newMatchTestWmeta(t))
		assertMatch(t, m, "app-ns", map[string]string{"app": "db"}, helm("default"))
		assertMatch(t, m, "ns", map[string]string{"app": "db"}, nothing)
	})

	t.Run("ssi on / enabledNamespaces / RC / last matching policy, else first matching target, else nothing", func(t *testing.T) {
		m := newMatchMutator(t, ssiOnEnabledNamespaces, newMatchTestWmeta(t))
		require.NoError(t, m.SetRemotePolicies(rcPolicies))
		assertMatch(t, m, "app-ns", map[string]string{"app": "legacy"}, nothing)
		assertMatch(t, m, "app-ns", map[string]string{"app": "other"}, rc("rc-default"))
		assertMatch(t, m, "ns", map[string]string{"app": "db"}, rc("rc-db"))
		assertMatch(t, m, "ns", map[string]string{"app": "legacy"}, nothing)
		assertMatch(t, m, "ns", map[string]string{"app": "other"}, rc("rc-default"))
	})

	t.Run("ssi on / targets / no RC / first matching target, else nothing", func(t *testing.T) {
		m := newMatchMutator(t, ssiOnTargets, newMatchTestWmeta(t))
		assertMatch(t, m, "ns", map[string]string{"language": "python"}, helm("helm-python"))
		assertMatch(t, m, "ns", map[string]string{"app": "db"}, nothing)
	})

	t.Run("ssi on / targets / RC / last matching policy, else first matching target, else nothing", func(t *testing.T) {
		m := newMatchMutator(t, ssiOnTargets, newMatchTestWmeta(t))
		require.NoError(t, m.SetRemotePolicies(rcPolicies))
		assertMatch(t, m, "ns", map[string]string{"language": "python"}, rc("rc-default"))
		assertMatch(t, m, "ns", map[string]string{"language": "python", "app": "db"}, rc("rc-db"))
		assertMatch(t, m, "ns", map[string]string{"app": "db"}, rc("rc-db"))
		assertMatch(t, m, "ns", map[string]string{"app": "legacy"}, nothing)
		assertMatch(t, m, "ns", map[string]string{"app": "other"}, rc("rc-default"))
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

// TestMatching_UnresolvableNamespaceRuleIsSkipped verifies that an unavailable
// namespace-label source only skips rules that need it. Other rules continue to
// be evaluated in order, independently of where the unresolvable rule appears.
func TestMatching_UnresolvableNamespaceRuleIsSkipped(t *testing.T) {
	const podRuleFirst = `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: "pod-only"
        podSelector:
          matchLabels:
            app: "web"
        ddTraceVersions:
          java: "default"
      - name: "ns-label"
        namespaceSelector:
          matchLabels:
            instrument: "true"
        ddTraceVersions:
          python: "default"
`
	const nsRuleFirst = `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: "ns-label"
        namespaceSelector:
          matchLabels:
            instrument: "true"
        ddTraceVersions:
          python: "default"
      - name: "pod-only"
        podSelector:
          matchLabels:
            app: "web"
        ddTraceVersions:
          java: "default"
`

	// No namespace is registered in the store, so the namespace-label rule
	// cannot be evaluated for the "ghost" namespace and is skipped.
	runMatchCases(t, podRuleFirst, []matchCase{
		{name: "pod-only rule before the unresolvable rule still matches", ns: "ghost", podLabels: map[string]string{"app": "web"}, want: "pod-only"},
	})
	runMatchCases(t, nsRuleFirst, []matchCase{
		{name: "unresolvable rule first does not block the pod-only rule", ns: "ghost", podLabels: map[string]string{"app": "web"}, want: "pod-only"},
	})
}
