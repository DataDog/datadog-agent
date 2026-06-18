// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package policies

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
)

// dd-wls JSON identifiers, matching the apm-policies.json schema enums.
const (
	nodeEvaluator = "EvaluatorNode"
	nodeComposite = "CompositeNode"

	evalString = "StrEvaluator"

	opAnd = "BOOL_AND"
	opOr  = "BOOL_OR"
	opNot = "BOOL_NOT"

	cmpExact    = "CMP_EXACT"
	cmpPrefix   = "CMP_PREFIX"
	cmpSuffix   = "CMP_SUFFIX"
	cmpContains = "CMP_CONTAINS"

	evalAlwaysTrue    = "ALWAYS_TRUE"
	evalAlwaysFalse   = "ALWAYS_FALSE"
	evalAlwaysAbstain = "ALWAYS_ABSTAIN"
	evalPodLabel      = "POD_LABEL"
	evalNamespaceLbl  = "NAMESPACE_LABEL"
	evalNamespaceName = "NAMESPACE_NAME"

	actionInjectAllow    = "INJECT_ALLOW"
	actionInjectDeny     = "INJECT_DENY"
	actionEnableSDK      = "ENABLE_SDK"
	actionEnableProfiler = "ENABLE_PROFILER"
)

type wlsPolicies struct {
	Policies []wlsPolicy `json:"policies"`
}

type wlsPolicy struct {
	Description string      `json:"description"`
	Rules       wlsNodeWrap `json:"rules"`
	Actions     []wlsAction `json:"actions"`
	ID          *wlsUUID    `json:"id"`
	Version     int64       `json:"version"`
}

// wlsUUID is the dd-wls 128-bit identifier, split into two unsigned longs
// because FlatBuffers cannot represent fixed-size byte arrays in Go.
type wlsUUID struct {
	Hi uint64 `json:"hi"`
	Lo uint64 `json:"lo"`
}

type wlsNodeWrap struct {
	NodeType string          `json:"node_type"`
	Node     json.RawMessage `json:"node"`
}

type wlsComposite struct {
	Description string        `json:"description"`
	Op          string        `json:"op"`
	Children    []wlsNodeWrap `json:"children"`
}

type wlsEvaluatorNode struct {
	Description string          `json:"description"`
	EvalType    string          `json:"eval_type"`
	Eval        json.RawMessage `json:"eval"`
}

type wlsStrEval struct {
	ID    string `json:"id"`
	Cmp   string `json:"cmp"`
	Value string `json:"value"`
}

type wlsAction struct {
	Action      string   `json:"action"`
	Description string   `json:"description"`
	Values      []string `json:"values"`
}

// ParsePolicies decodes a dd-wls policies document (the apm-policies.json
// format) into the native policy model. Label evaluators encode their key as
// "key=value" in the StrEvaluator value; a CMP_PREFIX on "key=" (empty value
// part) is decoded as an existence check.
func ParsePolicies(raw []byte) ([]Policy, error) {
	var doc wlsPolicies
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("invalid policies document: %w", err)
	}

	out := make([]Policy, 0, len(doc.Policies))
	for i, p := range doc.Policies {
		rules, err := decodeNodeWrap(p.Rules)
		if err != nil {
			return nil, fmt.Errorf("policy[%d] %q: %w", i, p.Description, err)
		}
		out = append(out, Policy{
			Name:    p.Description,
			ID:      decodeUUID(p.ID),
			Version: p.Version,
			Rules:   rules,
			Outcome: decodeActions(p.Actions),
		})
	}
	return out, nil
}

func decodeNodeWrap(w wlsNodeWrap) (*Node, error) {
	switch w.NodeType {
	case nodeComposite:
		var c wlsComposite
		if err := json.Unmarshal(w.Node, &c); err != nil {
			return nil, fmt.Errorf("invalid composite node: %w", err)
		}
		return decodeComposite(c)
	case nodeEvaluator:
		var e wlsEvaluatorNode
		if err := json.Unmarshal(w.Node, &e); err != nil {
			return nil, fmt.Errorf("invalid evaluator node: %w", err)
		}
		return decodeEvaluatorNode(e)
	default:
		return nil, fmt.Errorf("unsupported node_type %q", w.NodeType)
	}
}

func decodeComposite(c wlsComposite) (*Node, error) {
	children := make([]*Node, 0, len(c.Children))
	for _, child := range c.Children {
		n, err := decodeNodeWrap(child)
		if err != nil {
			return nil, err
		}
		children = append(children, n)
	}
	switch c.Op {
	case opAnd:
		return &Node{Op: OpAnd, Children: children}, nil
	case opOr:
		return &Node{Op: OpOr, Children: children}, nil
	case opNot:
		if len(children) != 1 {
			return nil, fmt.Errorf("BOOL_NOT requires exactly one child, got %d", len(children))
		}
		return &Node{Op: OpNot, Children: children}, nil
	default:
		return nil, fmt.Errorf("unsupported boolean operation %q", c.Op)
	}
}

func decodeEvaluatorNode(e wlsEvaluatorNode) (*Node, error) {
	if e.EvalType != evalString {
		return nil, fmt.Errorf("unsupported eval_type %q", e.EvalType)
	}
	var se wlsStrEval
	if err := json.Unmarshal(e.Eval, &se); err != nil {
		return nil, fmt.Errorf("invalid string evaluator: %w", err)
	}
	return decodeStrEval(se)
}

func decodeStrEval(e wlsStrEval) (*Node, error) {
	switch e.ID {
	case evalAlwaysTrue:
		return AlwaysTrue(), nil
	case evalAlwaysFalse:
		return AlwaysFalse(), nil
	case evalAlwaysAbstain:
		return AlwaysAbstain(), nil
	case evalPodLabel:
		return decodeLabel(SourcePodLabel, e)
	case evalNamespaceLbl:
		return decodeLabel(SourceNamespaceLabel, e)
	case evalNamespaceName:
		cmp, err := decodeCmp(e.Cmp)
		if err != nil {
			return nil, err
		}
		return Leaf(SourceNamespaceName, "", cmp, e.Value), nil
	default:
		return nil, fmt.Errorf("unsupported evaluator id %q", e.ID)
	}
}

func decodeLabel(src Source, e wlsStrEval) (*Node, error) {
	key, value, found := strings.Cut(e.Value, "=")
	if !found {
		return nil, fmt.Errorf("label evaluator value %q must be encoded as key=value", e.Value)
	}
	cmp, err := decodeCmp(e.Cmp)
	if err != nil {
		return nil, err
	}
	// "key=" with a prefix comparison is the existence convention.
	if cmp == CmpPrefix && value == "" {
		return Leaf(src, key, CmpExists, ""), nil
	}
	return Leaf(src, key, cmp, value), nil
}

func decodeCmp(cmp string) (Cmp, error) {
	switch cmp {
	case cmpExact:
		return CmpExact, nil
	case cmpPrefix:
		return CmpPrefix, nil
	case cmpSuffix:
		return CmpSuffix, nil
	case cmpContains:
		return CmpContains, nil
	default:
		return CmpExact, fmt.Errorf("unsupported string comparison %q", cmp)
	}
}

// decodeUUID renders the dd-wls hi/lo pair as a canonical UUID string. It
// returns an empty string when the document carries no id.
func decodeUUID(id *wlsUUID) string {
	if id == nil {
		return ""
	}
	var b [16]byte
	binary.BigEndian.PutUint64(b[0:8], id.Hi)
	binary.BigEndian.PutUint64(b[8:16], id.Lo)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func decodeActions(actions []wlsAction) Outcome {
	out := Outcome{}
	for _, a := range actions {
		switch a.Action {
		case actionInjectAllow:
			out.Inject = true
		case actionInjectDeny:
			out.Inject = false
		case actionEnableSDK:
			for _, v := range a.Values {
				lang, version, _ := strings.Cut(v, "=")
				if out.TracerVersions == nil {
					out.TracerVersions = map[string]string{}
				}
				out.TracerVersions[lang] = version
			}
		case actionEnableProfiler:
			out.TracerConfigs = append(out.TracerConfigs, EnvVar{Name: "DD_PROFILING_ENABLED", Value: "true"})
		}
	}
	return out
}
