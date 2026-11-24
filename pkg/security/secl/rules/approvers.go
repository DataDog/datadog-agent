// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"errors"
	"math"
	"net"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/pass"
)

// Approvers are just filter values indexed by field
type Approvers map[eval.Field]FilterValues

// ApproverStats is a struct that contains the stats of the approvers
type ApproverStats struct {
	// Field used as approver and the number of rules using it
	FieldStats map[eval.Field]int `json:"per_field_stats"`
	// Rule that is breaking the approver discovery
	AcceptModeRules map[eval.EventType]*Rule `json:"-"`
}

// NewApproverStats creates a new ApproverStats
func NewApproverStats() *ApproverStats {
	return &ApproverStats{
		FieldStats:      make(map[eval.Field]int),
		AcceptModeRules: make(map[eval.EventType]*Rule),
	}
}

// Merge merges two ApproverStats. They shouldn't be merged if they are for the same event type.
func (s *ApproverStats) Merge(other *ApproverStats) {
	if other == nil {
		return
	}

	for field, count := range other.FieldStats {
		s.FieldStats[field] += count
	}

	for eventType, rule := range other.AcceptModeRules {
		if _, exists := s.AcceptModeRules[eventType]; !exists {
			s.AcceptModeRules[eventType] = rule
		}
	}
}

// Approver is a test the kernel approves events on: it sends the events it
// approves to user space, and drops the others. It is comparable.
type Approver interface {
	// Weight is how selective the test is, the higher the fewer events it
	// approves.
	Weight() int
	// ApproverOnly is whether the events the test approves bypass the
	// discarders.
	ApproverOnly() bool
}

// reporter is an approver reporting itself rather than the comparison it
// approves.
type reporter interface {
	Report() (eval.FieldValueType, interface{})
}

// intersecter is an approver combining with another approver of its field into
// the approvers of the events both approve.
type intersecter interface {
	// Intersect returns the approvers of the events both approve, none if no
	// event is, false if it can't combine with the other approver.
	Intersect(other Approver) ([]Approver, bool)
}

// ApproverModel returns the approvers of a comparison C. For any event,
//
//	C is true  =>  A1 or ... or An approves it
//
// A1, ..., An being the approvers returned. The approvers may approve events C
// is false for. It returns false when the kernel can't approve events on C.
type ApproverModel func(c Comparison) ([]Approver, bool)

// Approvers returns the approvers of a comparison, false if the model has none:
// no approvers would mean that the comparison is never true.
func (m ApproverModel) Approvers(c Comparison) ([]Approver, bool) {
	approvers, ok := m(c)
	return approvers, ok && len(approvers) > 0
}

// On returns the model approving the comparisons of some fields as the model
// does. It has no approvers for the other fields.
func On(model ApproverModel, fields ...eval.Field) ApproverModel {
	return ApproverModel(func(c Comparison) ([]Approver, bool) {
		if !slices.Contains(fields, c.Field) {
			return nil, false
		}
		return model.Approvers(c)
	})
}

// Any returns the model approving a comparison as the most selective of the
// models having approvers for it.
func Any(models ...ApproverModel) ApproverModel {
	return ApproverModel(func(c Comparison) ([]Approver, bool) {
		var (
			best []Approver
			ok   bool
		)
		for _, model := range models {
			if approvers, approved := model.Approvers(c); approved && (!ok || moreSelective(approvers, best)) {
				best, ok = approvers, true
			}
		}
		return best, ok
	})
}

// moreSelective returns whether some approvers are more selective than others:
// approvers are as selective as the least selective of them, and the fewer the
// more selective.
func moreSelective[A interface{ Weight() int }](approvers, others []A) bool {
	weight, otherWeight := minWeight(approvers), minWeight(others)
	return weight > otherWeight || weight == otherWeight && len(approvers) < len(others)
}

// minWeight returns the weight of the least selective approver.
func minWeight[A interface{ Weight() int }](approvers []A) int {
	weight := math.MaxInt
	for _, approver := range approvers {
		weight = min(weight, approver.Weight())
	}
	return weight
}

// Comparison is a comparison of a field with a constant, `field op value`, or of
// some of its bits, `field & mask op value`.
type Comparison struct {
	Field eval.Field
	Op    ast.ScalarOp
	// Mask is the bits compared, -1 for the field itself
	Mask  int
	Value ast.Value
}

func (c Comparison) masked() bool {
	return c.Mask != -1
}

func (c Comparison) int() (int, bool) {
	if c.Value.Kind() != ast.Int {
		return 0, false
	}
	return ast.IntVal(c.Value), true
}

// Equal returns the string of `field == "value"`.
func (c Comparison) Equal() (string, bool) {
	if c.Op != ast.ScalarEqual || c.Value.Kind() != ast.String {
		return "", false
	}
	return ast.StringVal(c.Value), true
}

// Glob returns the pattern of `field =~ "pattern"`, `field == ~"pattern"`.
func (c Comparison) Glob() (string, bool) {
	isMatch := c.Op == ast.ScalarMatch && c.Value.Kind() == ast.String
	isPattern := (c.Op == ast.ScalarEqual || c.Op == ast.ScalarMatch) && c.Value.Kind() == ast.Pattern
	if !isMatch && !isPattern {
		return "", false
	}
	return ast.StringVal(c.Value), true
}

// Int returns the integer of `field == value`.
func (c Comparison) Int() (int, bool) {
	if c.Op != ast.ScalarEqual || c.masked() {
		return 0, false
	}
	return c.int()
}

// NotInt returns the integer of `field != value`.
func (c Comparison) NotInt() (int, bool) {
	if c.Op != ast.ScalarNotEqual || c.masked() {
		return 0, false
	}
	return c.int()
}

// Range returns the bounds of the integers an ordering or an equality is true
// for: [2, MaxInt] for `field > 1`.
func (c Comparison) Range() (int, int, bool) {
	value, ok := c.int()
	if !ok || c.masked() {
		return 0, 0, false
	}
	switch c.Op {
	case ast.ScalarEqual:
		return value, value, true
	case ast.ScalarLesser:
		return math.MinInt, value - 1, true
	case ast.ScalarLesserOrEqual:
		return math.MinInt, value, true
	case ast.ScalarGreater:
		return value + 1, math.MaxInt, true
	case ast.ScalarGreaterOrEqual:
		return value, math.MaxInt, true
	}
	return 0, 0, false
}

// AnyBits returns bits one of which is set when the comparison is true: the
// mask of `field & mask > 0`, `field & mask != 0`, the value of
// `field & mask == value` or `field == value`, value not zero.
func (c Comparison) AnyBits() (int, bool) {
	value, ok := c.int()
	if !ok {
		return 0, false
	}
	switch {
	case c.masked() && c.Mask > 0 && (c.Op == ast.ScalarGreater && value >= 0 ||
		c.Op == ast.ScalarGreaterOrEqual && value >= 1 ||
		c.Op == ast.ScalarNotEqual && value == 0):
		return c.Mask, true
	case c.Op == ast.ScalarEqual && value != 0:
		return value, true
	}
	return 0, false
}

// NoBits returns bits none of which is set when the comparison is true: the mask
// of `field & mask == 0`, all the bits, -1, for `field == 0`.
func (c Comparison) NoBits() (int, bool) {
	if value, ok := c.int(); !ok || value != 0 || c.Op != ast.ScalarEqual {
		return 0, false
	}
	return c.Mask, true
}

// Bool returns the boolean of `field == value`.
func (c Comparison) Bool() (bool, bool) {
	if c.Op != ast.ScalarEqual || c.Value.Kind() != ast.Bool {
		return false, false
	}
	return ast.BoolVal(c.Value), true
}

// IP returns the network of `field == 1.2.3.4`, `field in 10.0.0.0/8`.
func (c Comparison) IP() (net.IPNet, bool) {
	if kind := c.Value.Kind(); c.Op != ast.ScalarEqual || kind != ast.IP && kind != ast.CIDR {
		return net.IPNet{}, false
	}
	ipnet, err := eval.ParseCIDR(ast.StringVal(c.Value))
	if err != nil {
		return net.IPNet{}, false
	}
	return *ipnet, true
}

// isNever returns whether the comparison is never true: `field & mask == value`,
// value having bits out of the mask.
func (c Comparison) isNever() bool {
	value, ok := c.int()
	return ok && c.masked() && c.Op == ast.ScalarEqual && value&^c.Mask != 0
}

// report returns the comparison as the filter report shows it.
func (c Comparison) report() (eval.FieldValueType, interface{}) {
	if bits, ok := c.AnyBits(); ok && c.masked() {
		return eval.BitmaskValueType, bits
	}
	if minimum, maximum, ok := c.Range(); ok && c.Op != ast.ScalarEqual {
		return eval.RangeValueType, RangeFilterValue{Min: minimum, Max: maximum}
	}
	if pattern, ok := c.Glob(); ok {
		return eval.GlobValueType, pattern
	}
	switch c.Value.Kind() {
	case ast.Int:
		return eval.ScalarValueType, ast.IntVal(c.Value)
	case ast.Bool:
		return eval.ScalarValueType, ast.BoolVal(c.Value)
	case ast.IP, ast.CIDR:
		return eval.IPNetValueType, ast.StringVal(c.Value)
	}
	return eval.ScalarValueType, ast.StringVal(c.Value)
}

// ruleApprovers returns the approvers of a rule: an event none of them
// approves doesn't match the rule. It returns false if the rule has no
// approvers, the kernel having to send all the events of its type.
func ruleApprovers(rule *Rule, model ApproverModel) (FilterValues, bool, error) {
	tree := rule.GetResolvedAst()
	if tree == nil {
		return nil, false, errors.New("rule not compiled")
	}

	normalized, err := pass.Run(tree, pass.NegationNormalForm(), pass.FoldBools())
	if err != nil {
		return nil, false, err
	}

	approvers, ok := approversOf(normalized.(*ast.Rule).Root, model)
	return approvers, ok, nil
}

// approversOf returns the approvers of a boolean expression in negation normal
// form, false if it has none.
func approversOf(node ast.Node, model ApproverModel) (FilterValues, bool) {
	switch n := node.(type) {
	case *ast.LogicalExpr:
		left, leftOk := approversOf(n.Left, model)
		right, rightOk := approversOf(n.Right, model)
		if n.Op == ast.LogicalOr {
			return anyOf(left, leftOk, right, rightOk)
		}
		return allOf(left, leftOk, right, rightOk)
	case *ast.Literal:
		if n.Value.Kind() == ast.Bool && !ast.BoolVal(n.Value) {
			// never true, no event to approve
			return nil, true
		}
		return nil, false
	case *ast.UnaryExpr:
		if n.Op == ast.UnaryNot {
			return comparisonApprovers(n.Operand, true, model)
		}
	}
	return comparisonApprovers(node, false, model)
}

// anyOf returns the approvers of a disjunction: the approvers of both sides,
// none if one of them has none.
func anyOf(left FilterValues, leftOk bool, right FilterValues, rightOk bool) (FilterValues, bool) {
	if !leftOk || !rightOk {
		return nil, false
	}
	return append(left[:len(left):len(left)], right...), true
}

// allOf returns the approvers of a conjunction: the approvers of one side, the
// most selective one if both have approvers, or the intersection of both sides.
func allOf(left FilterValues, leftOk bool, right FilterValues, rightOk bool) (FilterValues, bool) {
	switch {
	case !leftOk:
		return right, rightOk
	case !rightOk:
		return left, true
	}
	if values, ok := intersect(left, right); ok {
		return values, true
	}
	if moreSelective(right, left) {
		return right, true
	}
	return left, true
}

// maxIntersections bounds the intersections of the approvers of the two sides of
// a conjunction, their number being the product of the numbers of approvers.
const maxIntersections = 16

// intersect returns the approvers of the events approved by both sides of a
// conjunction: the union of the intersections of the approvers of one side with
// the approvers of the other. It returns false if two of them can't be
// intersected, or if there are more than maxIntersections intersections.
func intersect(left, right FilterValues) (FilterValues, bool) {
	if len(left)*len(right) > maxIntersections {
		return nil, false
	}

	var values FilterValues
	for _, l := range left {
		approver, ok := l.Approver.(intersecter)
		if !ok {
			return nil, false
		}
		for _, r := range right {
			if l.Field != r.Field {
				return nil, false
			}
			approvers, ok := approver.Intersect(r.Approver)
			if !ok {
				return nil, false
			}
			for _, approver := range approvers {
				values = values.Merge(filterValue(l.Field, l.Type, l.Value, approver))
			}
		}
	}
	return values, true
}

// comparisonApprovers returns the approvers of a leaf of a boolean expression,
// of its negation if negated, false if it has none.
func comparisonApprovers(node ast.Node, negated bool, model ApproverModel) (FilterValues, bool) {
	switch n := node.(type) {
	case *ast.FieldRef:
		if isApprovable(n) {
			return approve(model, Comparison{Field: n.Name, Op: ast.ScalarEqual, Mask: -1, Value: ast.MakeBool(!negated)})
		}
	case *ast.ScalarCompareExpr:
		if !negated {
			return scalarCompareApprovers(n, model)
		}
	case *ast.ArrayCompareExpr:
		if !negated {
			return arrayCompareApprovers(n, model)
		}
	}
	return nil, false
}

// approve returns the approvers of a comparison, as filter values of its field.
func approve(model ApproverModel, c Comparison) (FilterValues, bool) {
	if c.isNever() {
		return nil, true
	}

	approvers, ok := model.Approvers(c)
	if !ok {
		return nil, false
	}

	valueType, value := c.report()
	values := make(FilterValues, 0, len(approvers))
	for _, approver := range approvers {
		values = append(values, filterValue(c.Field, valueType, value, approver))
	}
	return values, true
}

// filterValue returns the filter value of an approver of a comparison.
func filterValue(field eval.Field, valueType eval.FieldValueType, value interface{}, approver Approver) FilterValue {
	fv := FilterValue{Field: field, Value: value, Type: valueType, Mode: DefaultMode, Approver: approver}
	if r, ok := approver.(reporter); ok {
		fv.Type, fv.Value = r.Report()
	}
	if approver.ApproverOnly() {
		fv.Mode = ApproverOnlyMode
	}
	return fv
}

// isApprovable returns whether events can be approved on a field: it is read
// from the event, not from one of the elements of an iteration.
func isApprovable(field *ast.FieldRef) bool {
	return field.Iter == nil
}

// operand is the field side of a comparison: a field, or the bits of a field,
// `field & mask`, mask being -1 for the field itself.
type operand struct {
	field eval.Field
	mask  int
}

// operandOf returns the operand of a comparison side, false if it isn't one.
func operandOf(node ast.Node) (operand, bool) {
	switch n := node.(type) {
	case *ast.FieldRef:
		if isApprovable(n) {
			return operand{field: n.Name, mask: -1}, true
		}
	case *ast.BitExpr:
		if n.Op != ast.BitAnd {
			break
		}
		for _, sides := range [2][2]ast.Node{{n.Left, n.Right}, {n.Right, n.Left}} {
			field, ok := sides[0].(*ast.FieldRef)
			if !ok || !isApprovable(field) {
				continue
			}
			if mask, ok := ast.IntConstant(sides[1]); ok {
				return operand{field: field.Name, mask: mask}, true
			}
		}
	}
	return operand{}, false
}

// mirroredScalarOps are the comparisons of the operands swapped: `a < b` is
// `b > a`.
var mirroredScalarOps = map[ast.ScalarOp]ast.ScalarOp{
	ast.ScalarLesser:         ast.ScalarGreater,
	ast.ScalarLesserOrEqual:  ast.ScalarGreaterOrEqual,
	ast.ScalarGreater:        ast.ScalarLesser,
	ast.ScalarGreaterOrEqual: ast.ScalarLesserOrEqual,
}

// scalarCompareApprovers returns the approvers of a comparison of a field with a
// value, false if the comparison isn't one.
func scalarCompareApprovers(node *ast.ScalarCompareExpr, model ApproverModel) (FilterValues, bool) {
	op, value := node.Op, node.Right
	field, ok := operandOf(node.Left)
	if !ok {
		if field, ok = operandOf(node.Right); !ok {
			return nil, false
		}
		value = node.Left
		if mirrored, ok := mirroredScalarOps[op]; ok {
			op = mirrored
		}
	}
	return valueApprovers(field, op, value, model)
}

// arrayCompareApprovers returns the approvers of a membership test of a field:
// the approvers of `field == a || field == b` for `field in [a, b]`, of
// `field != a && field != b` for `field not in [a, b]`.
func arrayCompareApprovers(node *ast.ArrayCompareExpr, model ApproverModel) (FilterValues, bool) {
	field, ok := operandOf(node.Left)
	if !ok {
		return nil, false
	}

	var members []*ast.Literal
	switch n := node.Right.(type) {
	case *ast.Array:
		members = n.Members
	case *ast.Literal:
		// a lone CIDR is an array of one CIDR
		if n.Value.Kind() != ast.CIDR {
			return nil, false
		}
		members = []*ast.Literal{n}
	default:
		return nil, false
	}

	var (
		approvers FilterValues
		combine   func(FilterValues, bool, FilterValues, bool) (FilterValues, bool)
		op        ast.ScalarOp
	)
	switch node.Op {
	case ast.ArrayIn:
		// the approvers of `false`, the empty disjunction
		approvers, ok, combine, op = nil, true, anyOf, ast.ScalarEqual
	case ast.ArrayNotIn:
		// the approvers of `true`, the empty conjunction
		approvers, ok, combine, op = nil, false, allOf, ast.ScalarNotEqual
	default:
		return nil, false
	}

	for _, member := range members {
		memberApprovers, memberOk := valueApprovers(field, op, member, model)
		approvers, ok = combine(approvers, ok, memberApprovers, memberOk)
	}
	return approvers, ok
}

// valueApprovers returns the approvers of a comparison of an operand with a
// value, false if the value isn't a constant.
func valueApprovers(field operand, op ast.ScalarOp, value ast.Node, model ApproverModel) (FilterValues, bool) {
	c := Comparison{Field: field.field, Op: op, Mask: field.mask}
	if i, ok := ast.IntConstant(value); ok {
		c.Value = ast.MakeInt(i)
		return approve(model, c)
	}

	literal, ok := value.(*ast.Literal)
	if !ok || field.mask != -1 {
		return nil, false
	}
	c.Value = literal.Value

	// `field != true` is `field == false`
	if literal.Value.Kind() == ast.Bool && op == ast.ScalarNotEqual {
		c.Op, c.Value = ast.ScalarEqual, ast.MakeBool(!ast.BoolVal(literal.Value))
	}
	return approve(model, c)
}

// getApprovers returns the approvers of rules of the same event type, the rules
// not to use for discarders, and nil if one of the rules has no approvers.
func getApprovers(rules []*Rule, model ApproverModel) (Approvers, *ApproverStats, []*Rule, error) {
	var (
		approvers = make(Approvers)
		stats     = NewApproverStats()
		// Rules that are not used by the discarder mechanism
		noDiscarderRules []*Rule
	)

	for _, rule := range rules {
		eventType, err := rule.GetEventType()
		if err != nil {
			return nil, stats, nil, err
		}

		ruleApprovers, ok, err := ruleApprovers(rule, model)
		if err != nil {
			stats.AcceptModeRules[eventType] = rule
			return nil, stats, nil, err
		}

		// no approvers for a rule thus no approver for the event type
		if !ok {
			stats.AcceptModeRules[eventType] = rule
			return nil, stats, nil, nil
		}

		var (
			fields       = make(map[eval.Field]bool)
			approverOnly = len(ruleApprovers) > 0
		)
		for _, approver := range ruleApprovers {
			approvers[approver.Field] = approvers[approver.Field].Merge(approver)
			fields[approver.Field] = true
			approverOnly = approverOnly && approver.Mode == ApproverOnlyMode
		}
		for field := range fields {
			stats.FieldStats[field]++
		}

		// The events approved by an approver in ApproverOnly mode are immune to the discarders, but those of the fields the
		// rule set keeps the excluded rules for: a rule all the approvers of which are in this mode can be excluded from the
		// discovery of the other discarders, they never drop its events. One approver in another mode would let the
		// discarders drop the events it approves.
		// ex: open.file.name != "" && process.auid == 1000 # this rule avoid open.file.path discarder discovery, but with a ApproverOnly on process.auid the problem disappear
		//     open.file.filename == "/etc/passwd"
		if approverOnly {
			noDiscarderRules = append(noDiscarderRules, rule)
		}
	}

	return approvers, stats, noDiscarderRules, nil
}
