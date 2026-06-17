// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package policies

import "testing"

func mustParse(t *testing.T, raw string) []Policy {
	t.Helper()
	ps, err := ParsePolicies([]byte(raw))
	if err != nil {
		t.Fatalf("ParsePolicies: %v", err)
	}
	return ps
}

func TestParsePodLabelAndActions(t *testing.T) {
	raw := `{
      "policies": [{
        "description": "java for db-user",
        "rules": {
          "node_type": "EvaluatorNode",
          "node": {
            "eval_type": "StrEvaluator",
            "eval": {"id": "POD_LABEL", "cmp": "CMP_EXACT", "value": "app=db-user"}
          }
        },
        "actions": [
          {"action": "INJECT_ALLOW"},
          {"action": "ENABLE_SDK", "values": ["java=latest"]}
        ]
      }]
    }`
	ps := mustParse(t, raw)
	if len(ps) != 1 || ps[0].Name != "java for db-user" {
		t.Fatalf("unexpected parse: %+v", ps)
	}

	out, ok := Decide(ps, Facts{PodLabels: map[string]string{"app": "db-user"}})
	if !ok || !out.Inject || out.TracerVersions["java"] != "latest" {
		t.Fatalf("db-user pod: %+v ok=%v", out, ok)
	}
	if _, ok := Decide(ps, Facts{PodLabels: map[string]string{"app": "other"}}); ok {
		t.Fatalf("non-matching pod should not match")
	}
}

func TestParseInjectDeny(t *testing.T) {
	raw := `{
      "policies": [{
        "description": "deny app=legacy",
        "rules": {"node_type": "EvaluatorNode", "node": {"eval_type": "StrEvaluator",
          "eval": {"id": "POD_LABEL", "cmp": "CMP_EXACT", "value": "app=legacy"}}},
        "actions": [{"action": "INJECT_DENY"}]
      }]
    }`
	ps := mustParse(t, raw)
	out, ok := Decide(ps, Facts{PodLabels: map[string]string{"app": "legacy"}})
	if !ok {
		t.Fatalf("policy should match")
	}
	if out.Inject {
		t.Fatalf("matched deny policy must not inject")
	}
}

func TestParseExistenceAndNot(t *testing.T) {
	// tier Exists (CMP_PREFIX "tier=") AND NOT deprecated Exists
	raw := `{
      "policies": [{
        "description": "exists",
        "rules": {
          "node_type": "CompositeNode",
          "node": {
            "op": "BOOL_AND",
            "children": [
              {"node_type": "EvaluatorNode", "node": {"eval_type": "StrEvaluator",
                "eval": {"id": "POD_LABEL", "cmp": "CMP_PREFIX", "value": "tier="}}},
              {"node_type": "CompositeNode", "node": {"op": "BOOL_NOT", "children": [
                {"node_type": "EvaluatorNode", "node": {"eval_type": "StrEvaluator",
                  "eval": {"id": "POD_LABEL", "cmp": "CMP_PREFIX", "value": "deprecated="}}}
              ]}}
            ]
          }
        },
        "actions": [{"action": "INJECT_ALLOW"}]
      }]
    }`
	ps := mustParse(t, raw)

	if _, ok := Decide(ps, Facts{PodLabels: map[string]string{"tier": "frontend"}}); !ok {
		t.Errorf("tier present, deprecated absent should match")
	}
	if _, ok := Decide(ps, Facts{PodLabels: map[string]string{}}); ok {
		t.Errorf("tier absent should not match")
	}
	if _, ok := Decide(ps, Facts{PodLabels: map[string]string{"tier": "x", "deprecated": "true"}}); ok {
		t.Errorf("deprecated present should not match")
	}
}

func TestParseNamespaceNameAndDefault(t *testing.T) {
	raw := `{
      "policies": [
        {
          "description": "ns names",
          "rules": {"node_type": "CompositeNode", "node": {"op": "BOOL_OR", "children": [
            {"node_type": "EvaluatorNode", "node": {"eval_type": "StrEvaluator",
              "eval": {"id": "NAMESPACE_NAME", "cmp": "CMP_EXACT", "value": "payments"}}},
            {"node_type": "EvaluatorNode", "node": {"eval_type": "StrEvaluator",
              "eval": {"id": "NAMESPACE_NAME", "cmp": "CMP_EXACT", "value": "billing"}}}
          ]}},
          "actions": [{"action": "INJECT_ALLOW"}, {"action": "ENABLE_SDK", "values": ["java=latest"]}]
        },
        {
          "description": "default",
          "rules": {"node_type": "EvaluatorNode", "node": {"eval_type": "StrEvaluator",
            "eval": {"id": "ALWAYS_TRUE", "cmp": "CMP_EXACT", "value": ""}}},
          "actions": [{"action": "INJECT_ALLOW"}]
        }
      ]
    }`
	ps := mustParse(t, raw)
	if len(ps) != 2 {
		t.Fatalf("expected 2 policies, got %d", len(ps))
	}

	out, ok := Decide(ps, Facts{NamespaceName: "billing"})
	if !ok || out.TracerVersions["java"] != "latest" {
		t.Fatalf("billing should hit first policy: %+v ok=%v", out, ok)
	}
	out, ok = Decide(ps, Facts{NamespaceName: "default"})
	if !ok || !out.Inject || len(out.TracerVersions) != 0 {
		t.Fatalf("default ns should hit catch-all: %+v ok=%v", out, ok)
	}
}

func TestParseUUID(t *testing.T) {
	raw := `{
      "policies": [{
        "description": "with id",
        "id": {"hi": 10, "lo": 11},
        "version": 7,
        "rules": {"node_type": "EvaluatorNode", "node": {"eval_type": "StrEvaluator",
          "eval": {"id": "ALWAYS_TRUE", "cmp": "CMP_EXACT", "value": ""}}},
        "actions": [{"action": "INJECT_ALLOW"}]
      }]
    }`
	ps := mustParse(t, raw)
	if ps[0].ID != "00000000-0000-000a-0000-00000000000b" {
		t.Errorf("unexpected UUID: %q", ps[0].ID)
	}
	if ps[0].Version != 7 {
		t.Errorf("unexpected version: %d", ps[0].Version)
	}

	// No id field => empty ID.
	noID := mustParse(t, `{"policies":[{"description":"x","rules":{"node_type":"EvaluatorNode","node":{"eval_type":"StrEvaluator","eval":{"id":"ALWAYS_TRUE"}}},"actions":[{"action":"INJECT_ALLOW"}]}]}`)
	if noID[0].ID != "" {
		t.Errorf("expected empty ID, got %q", noID[0].ID)
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"bad json":         `{`,
		"unknown id":       `{"policies":[{"rules":{"node_type":"EvaluatorNode","node":{"eval_type":"StrEvaluator","eval":{"id":"POD_ANNOTATION","cmp":"CMP_EXACT","value":"a=b"}}}}]}`,
		"label no equals":  `{"policies":[{"rules":{"node_type":"EvaluatorNode","node":{"eval_type":"StrEvaluator","eval":{"id":"POD_LABEL","cmp":"CMP_EXACT","value":"app"}}}}]}`,
		"not too many":     `{"policies":[{"rules":{"node_type":"CompositeNode","node":{"op":"BOOL_NOT","children":[{"node_type":"EvaluatorNode","node":{"eval_type":"StrEvaluator","eval":{"id":"ALWAYS_TRUE"}}},{"node_type":"EvaluatorNode","node":{"eval_type":"StrEvaluator","eval":{"id":"ALWAYS_TRUE"}}}]}}}]}`,
		"unknown node":     `{"policies":[{"rules":{"node_type":"Mystery","node":{}}}]}`,
		"unsupported eval": `{"policies":[{"rules":{"node_type":"EvaluatorNode","node":{"eval_type":"NumEvaluator","eval":{}}}}]}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParsePolicies([]byte(raw)); err == nil {
				t.Errorf("expected error for %q", name)
			}
		})
	}
}
