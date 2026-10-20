// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package jq

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strings"
	"unicode/utf8"
)

var (
	errEvaluationLimit = errors.New("evaluation step limit exceeded")
	errResultLimit     = errors.New("evaluation result limit exceeded")
	errRetentionLimit  = errors.New("evaluation retained-value limit exceeded")
)

type runtimeError struct{ message string }

func (e *runtimeError) Error() string { return e.message }

func runtimeErrorf(format string, args ...any) error {
	return &runtimeError{message: fmt.Sprintf(format, args...)}
}

type evaluator struct {
	ctx        context.Context
	variables  map[string]value
	retention  evaluationRetention
	steps      int
	topResults int
}

func newEvaluator(ctx context.Context, variables map[string]value) *evaluator {
	return &evaluator{ctx: ctx, variables: variables}
}

func (e *evaluator) evaluate(input value, root *node) ([]value, error) {
	results, err := e.eval(root, input)
	if e.retention.nodes != 0 || e.retention.bytes != 0 {
		return nil, errors.New("internal evaluation retention accounting imbalance")
	}
	bounded, boundErr := e.bounded(results)
	if boundErr != nil {
		return bounded, boundErr
	}
	remaining := MaxResults - e.topResults
	if len(bounded) > remaining {
		e.topResults += remaining
		return bounded[:remaining], errResultLimit
	}
	e.topResults += len(bounded)
	return bounded, err
}

func (e *evaluator) tick() error {
	if err := e.ctx.Err(); err != nil {
		return err
	}
	e.steps++
	if e.steps > MaxEvalSteps {
		return errEvaluationLimit
	}
	return nil
}

func (e *evaluator) bounded(results []value) ([]value, error) {
	acc := newResultAccumulator(len(results))
	if err := acc.addAll(results); err != nil {
		return acc.values, err
	}
	return acc.values, nil
}

type resultAccumulator struct {
	values    []value
	retention *evaluationRetention
	nodes     int
	bytes     int
}

func newResultAccumulator(capacity int) *resultAccumulator {
	if capacity > MaxResults {
		capacity = MaxResults
	}
	return &resultAccumulator{values: make([]value, 0, capacity)}
}

func (e *evaluator) newResultAccumulator(capacity int) *resultAccumulator {
	acc := newResultAccumulator(capacity)
	acc.retention = &e.retention
	return acc
}

func (a *resultAccumulator) add(v value) error {
	if len(a.values) >= MaxResults || v.nodes > MaxResultNodes-a.nodes || v.bytes > MaxResultBytes-a.bytes {
		return errResultLimit
	}
	if a.retention != nil {
		if err := a.retention.retain(v); err != nil {
			return err
		}
	}
	a.values = append(a.values, v)
	a.nodes += v.nodes
	a.bytes += v.bytes
	return nil
}

func (a *resultAccumulator) release() {
	if a.retention == nil {
		return
	}
	a.retention.release(a.values...)
	a.retention = nil
}

func (a *resultAccumulator) addAll(values []value) error {
	for _, v := range values {
		if err := a.add(v); err != nil {
			return err
		}
	}
	return nil
}

func (e *evaluator) evalField(n *node, input value, optional bool) ([]value, error) {
	bases, baseErr := e.eval(n.left, input)
	results := newResultAccumulator(len(bases))
	for _, base := range bases {
		if err := e.tick(); err != nil {
			return results.values, err
		}
		v, err := fieldValue(base, n.name)
		if err != nil {
			if optional {
				continue
			}
			return results.values, err
		}
		if err := results.add(v); err != nil {
			return results.values, err
		}
	}
	return results.values, baseErr
}

func (e *evaluator) evalIndex(n *node, input value, optional bool) ([]value, error) {
	indexes, indexErr := e.eval(n.right, input)
	if err := e.retention.retain(indexes...); err != nil {
		return nil, err
	}
	defer func() {
		e.retention.release(indexes...)
	}()
	results := e.newResultAccumulator(0)
	defer results.release()
	// jq applies each index to every base before advancing to the next index.
	// Re-evaluation avoids retaining both generator batches at once.
	for i := range indexes {
		index := indexes[i]
		bases, baseErr := e.eval(n.left, input)
		for _, base := range bases {
			if err := e.tick(); err != nil {
				return results.values, err
			}
			v, err := indexValue(base, index)
			if err != nil {
				if optional {
					continue
				}
				return results.values, err
			}
			if err := results.add(v); err != nil {
				return results.values, err
			}
		}
		clear(bases)
		bases = nil
		indexes[i] = value{}
		e.retention.release(index)
		if baseErr != nil {
			return results.values, baseErr
		}
	}
	return results.values, indexErr
}

func (e *evaluator) evalIterator(n *node, input value, optional bool) ([]value, error) {
	bases, baseErr := e.eval(n.left, input)
	results := newResultAccumulator(0)
	for _, base := range bases {
		if err := e.tick(); err != nil {
			return results.values, err
		}
		switch base.kind {
		case valueArray:
			if err := results.addAll(base.array); err != nil {
				return results.values, err
			}
		case valueObject:
			for _, member := range base.obj.members {
				if err := results.add(member.value); err != nil {
					return results.values, err
				}
			}
		default:
			if !optional {
				return results.values, runtimeErrorf("cannot iterate over %s", typeName(base))
			}
		}
	}
	return results.values, baseErr
}

func (e *evaluator) evalOptional(child *node, input value) ([]value, error) {
	switch child.kind {
	case nodeField:
		return e.evalField(child, input, true)
	case nodeIndex:
		return e.evalIndex(child, input, true)
	case nodeIterator:
		return e.evalIterator(child, input, true)
	default:
		results, err := e.eval(child, input)
		if err == nil {
			return results, nil
		}
		var runtimeErr *runtimeError
		if errors.As(err, &runtimeErr) {
			return e.bounded(results)
		}
		return results, err
	}
}

func (e *evaluator) eval(n *node, input value) ([]value, error) {
	if err := e.tick(); err != nil {
		return nil, err
	}
	switch n.kind {
	case nodeIdentity:
		return []value{input}, nil
	case nodeLiteral:
		return []value{n.literal}, nil
	case nodeVariable:
		v, ok := e.variables[n.name]
		if !ok {
			return nil, runtimeErrorf("variable $%s is not defined", n.name)
		}
		return []value{v}, nil
	case nodeField:
		return e.evalField(n, input, false)
	case nodeIndex:
		return e.evalIndex(n, input, false)
	case nodeIterator:
		return e.evalIterator(n, input, false)
	case nodeGroup:
		return e.eval(n.child, input)
	case nodeOptional:
		return e.evalOptional(n.child, input)
	case nodePipe:
		left, leftErr := e.eval(n.left, input)
		if err := e.retention.retain(left...); err != nil {
			return nil, err
		}
		defer func() {
			e.retention.release(left...)
		}()
		results := e.newResultAccumulator(0)
		defer results.release()
		for i := range left {
			item := left[i]
			right, err := e.eval(n.right, item)
			if addErr := results.addAll(right); addErr != nil {
				return results.values, addErr
			}
			clear(right)
			right = nil
			left[i] = value{}
			e.retention.release(item)
			if err != nil {
				return results.values, err
			}
		}
		return results.values, leftErr
	case nodeComma:
		left, err := e.eval(n.left, input)
		if err != nil {
			return left, err
		}
		if err := e.retention.retain(left...); err != nil {
			return left, err
		}
		leftRetained := true
		defer func() {
			if leftRetained {
				e.retention.release(left...)
			}
		}()
		right, err := e.eval(n.right, input)
		e.retention.release(left...)
		leftRetained = false
		results := newResultAccumulator(len(left) + len(right))
		if addErr := results.addAll(left); addErr != nil {
			return results.values, addErr
		}
		if addErr := results.addAll(right); addErr != nil {
			return results.values, addErr
		}
		return results.values, err
	case nodeAlternative:
		left, leftErr := e.eval(n.left, input)
		results := newResultAccumulator(len(left))
		for _, item := range left {
			if truthy(item) {
				if err := results.add(item); err != nil {
					return results.values, err
				}
			}
		}
		if leftErr != nil {
			return results.values, leftErr
		}
		if len(results.values) > 0 {
			return results.values, nil
		}
		clear(left)
		left = nil
		results = nil
		return e.eval(n.right, input)
	case nodeBinary:
		return e.evalBinary(n, input)
	case nodeNegate:
		items, childErr := e.eval(n.child, input)
		results := newResultAccumulator(len(items))
		for _, item := range items {
			v, err := negateNumber(item)
			if err != nil {
				return results.values, err
			}
			if err := results.add(v); err != nil {
				return results.values, err
			}
		}
		return results.values, childErr
	case nodeArray:
		items := []value(nil)
		if n.child != nil {
			var err error
			items, err = e.eval(n.child, input)
			if err != nil {
				return nil, err
			}
		}
		v, err := arrayValue(items)
		if err != nil {
			return nil, err
		}
		return []value{v}, nil
	case nodeObject:
		return e.evalObject(n, input)
	case nodeCall:
		return e.evalCall(n, input)
	default:
		return nil, errors.New("invalid filter node")
	}
}

func (e *evaluator) evalBinary(n *node, input value) ([]value, error) {
	if n.name != "and" && n.name != "or" {
		return e.evalOrdinaryBinary(n, input)
	}
	left, leftErr := e.eval(n.left, input)
	if err := e.retention.retain(left...); err != nil {
		return nil, err
	}
	defer func() {
		e.retention.release(left...)
	}()
	results := e.newResultAccumulator(0)
	defer results.release()
	for i := range left {
		leftValue := left[i]
		if n.name == "and" && !truthy(leftValue) {
			if err := results.add(boolean(false)); err != nil {
				return results.values, err
			}
			left[i] = value{}
			e.retention.release(leftValue)
			continue
		}
		if n.name == "or" && truthy(leftValue) {
			if err := results.add(boolean(true)); err != nil {
				return results.values, err
			}
			left[i] = value{}
			e.retention.release(leftValue)
			continue
		}

		right, rightErr := e.eval(n.right, input)
		for _, rightValue := range right {
			if err := e.tick(); err != nil {
				return results.values, err
			}
			if err := results.add(boolean(truthy(rightValue))); err != nil {
				return results.values, err
			}
		}
		clear(right)
		right = nil
		left[i] = value{}
		e.retention.release(leftValue)
		if rightErr != nil {
			return results.values, rightErr
		}
	}
	return results.values, leftErr
}

func (e *evaluator) evalOrdinaryBinary(n *node, input value) ([]value, error) {
	right, rightErr := e.eval(n.right, input)
	if err := e.retention.retain(right...); err != nil {
		return nil, err
	}
	defer func() {
		e.retention.release(right...)
	}()
	results := e.newResultAccumulator(0)
	defer results.release()
	for i := range right {
		rightValue := right[i]
		left, err := e.eval(n.left, input)
		if applyErr := e.applyBinaryBatch(n.op, left, rightValue, results); applyErr != nil {
			return results.values, applyErr
		}
		clear(left)
		left = nil
		right[i] = value{}
		e.retention.release(rightValue)
		if err != nil {
			return results.values, err
		}
	}
	return results.values, rightErr
}

func (e *evaluator) applyBinaryBatch(op tokenKind, left []value, right value, results *resultAccumulator) error {
	if err := e.retention.retain(left...); err != nil {
		return err
	}
	defer e.retention.release(left...)
	for _, leftValue := range left {
		if err := e.tick(); err != nil {
			return err
		}
		result, err := e.applyBinary(op, leftValue, right)
		if err != nil {
			return err
		}
		if err := results.add(result); err != nil {
			return err
		}
	}
	return nil
}

type evaluationRetention struct {
	nodes int
	bytes int
}

func (r *evaluationRetention) retainSize(nodes, size int) error {
	if nodes > MaxResultNodes-r.nodes || size > MaxResultBytes-r.bytes {
		return errRetentionLimit
	}
	r.nodes += nodes
	r.bytes += size
	return nil
}

func (r *evaluationRetention) releaseSize(nodes, size int) {
	r.nodes -= nodes
	r.bytes -= size
}

func (r *evaluationRetention) retain(values ...value) error {
	nodes, size := r.nodes, r.bytes
	for _, item := range values {
		if item.nodes > MaxResultNodes-nodes || item.bytes > MaxResultBytes-size {
			return errRetentionLimit
		}
		nodes += item.nodes
		size += item.bytes
	}
	r.nodes = nodes
	r.bytes = size
	return nil
}

func (r *evaluationRetention) release(values ...value) {
	for _, item := range values {
		r.nodes -= item.nodes
		r.bytes -= item.bytes
	}
}

func (e *evaluator) evalObject(n *node, input value) ([]value, error) {
	results := e.newResultAccumulator(0)
	defer results.release()
	path := make([]objectMember, 0, len(n.members))
	keyIndex := make(map[string]int, len(n.members))
	remainingStaticKeys := make(map[string]int, len(n.members))
	if err := e.retention.retainSize(1, 2); err != nil {
		return nil, err
	}
	defer e.retention.releaseSize(1, 2)
	for _, member := range n.members {
		if key, ok := staticObjectKey(member); ok {
			remainingStaticKeys[key]++
		}
	}
	err := e.evalObjectMembers(n.members, 0, &path, keyIndex, remainingStaticKeys, 1, 2, input, results)
	return results.values, err
}

func staticObjectKey(member objectNodeMember) (string, bool) {
	if member.dynamicKey == nil {
		return member.literalKey, true
	}
	key := member.dynamicKey
	for key != nil && key.kind == nodeGroup {
		key = key.child
	}
	if key != nil && key.kind == nodeLiteral && key.literal.kind == valueString {
		return key.literal.str, true
	}
	return "", false
}

func (e *evaluator) evalObjectMembers(
	members []objectNodeMember,
	memberIndex int,
	path *[]objectMember,
	keyIndex map[string]int,
	remainingStaticKeys map[string]int,
	nodes int,
	size int,
	input value,
	results *resultAccumulator,
) error {
	if memberIndex == len(members) {
		item, err := object(*path)
		if err != nil {
			return err
		}
		return results.add(item)
	}

	memberNode := members[memberIndex]
	if key, ok := staticObjectKey(memberNode); ok {
		remainingStaticKeys[key]--
		defer func() {
			remainingStaticKeys[key]++
		}()
	}
	// jq does not coerce a member key when its value is empty.
	processKey := func(resolveKey func() (string, error)) error {
		values, valueErr := e.eval(memberNode.value, input)
		if len(values) == 0 {
			return valueErr
		}
		key, err := resolveKey()
		if err != nil {
			return err
		}
		if existing, ok := keyIndex[key]; ok {
			key = (*path)[existing].key
		}
		processValue := func(stored value) error {
			nextNodes, nextSize := nodes, size
			if existing, ok := keyIndex[key]; ok {
				previous := (*path)[existing].value
				nextNodes -= previous.nodes
				nextSize -= previous.bytes
				if err := addAggregate(&nextNodes, &nextSize, stored, 0, MaxValueNodes, MaxValueBytes); err != nil {
					return err
				}
				if err := e.retention.retain(stored); err != nil {
					return err
				}
				(*path)[existing].value = stored
				err := e.evalObjectMembers(members, memberIndex+1, path, keyIndex, remainingStaticKeys, nextNodes, nextSize, input, results)
				(*path)[existing].value = previous
				e.retention.release(stored)
				return err
			}

			separator := 0
			if len(*path) > 0 {
				separator = 1
			}
			extra := len(key) + 3 + separator
			if err := addAggregate(&nextNodes, &nextSize, stored, extra, MaxValueNodes, MaxValueBytes); err != nil {
				return err
			}
			if err := e.retention.retainSize(stored.nodes, stored.bytes+extra); err != nil {
				return err
			}
			keyIndex[key] = len(*path)
			*path = append(*path, objectMember{key: key, value: stored})
			err := e.evalObjectMembers(members, memberIndex+1, path, keyIndex, remainingStaticKeys, nextNodes, nextSize, input, results)
			*path = (*path)[:len(*path)-1]
			delete(keyIndex, key)
			e.retention.releaseSize(stored.nodes, stored.bytes+extra)
			return err
		}

		if remainingStaticKeys[key] > 0 {
			branchCount := len(values)
			clear(values)
			values = nil
			for range branchCount {
				if err := e.tick(); err != nil {
					return err
				}
				if err := processValue(null()); err != nil {
					return err
				}
			}
			return valueErr
		}

		if err := e.retention.retain(values...); err != nil {
			return err
		}
		defer func() {
			e.retention.release(values...)
		}()
		for i := range values {
			item := values[i]
			if err := e.tick(); err != nil {
				return err
			}
			values[i] = value{}
			e.retention.release(item)
			if err := processValue(item); err != nil {
				return err
			}
		}
		return valueErr
	}

	if memberNode.dynamicKey == nil {
		return processKey(func() (string, error) { return memberNode.literalKey, nil })
	}
	keyValues, keyErr := e.eval(memberNode.dynamicKey, input)
	if err := e.retention.retain(keyValues...); err != nil {
		return err
	}
	defer func() {
		e.retention.release(keyValues...)
	}()
	for i := range keyValues {
		keyValue := keyValues[i]
		err := processKey(func() (string, error) {
			if keyValue.kind != valueString {
				return "", runtimeErrorf("object keys must be strings, not %s", typeName(keyValue))
			}
			return keyValue.str, nil
		})
		keyValues[i] = value{}
		e.retention.release(keyValue)
		keyValue = value{}
		if err != nil {
			return err
		}
	}
	return keyErr
}

func (e *evaluator) evalCall(n *node, input value) ([]value, error) {
	switch n.name {
	case "empty":
		return nil, nil
	case "not":
		return []value{boolean(!truthy(input))}, nil
	case "type":
		v, _ := stringValue(typeName(input))
		return []value{v}, nil
	case "length":
		v, err := lengthValue(input)
		if err != nil {
			return nil, err
		}
		return []value{v}, nil
	case "keys":
		v, err := keysValue(input)
		if err != nil {
			return nil, err
		}
		return []value{v}, nil
	case "has":
		keys, keyErr := e.eval(n.child, input)
		results := newResultAccumulator(len(keys))
		for _, key := range keys {
			has, err := hasValue(input, key)
			if err != nil {
				return results.values, err
			}
			if err := results.add(boolean(has)); err != nil {
				return results.values, err
			}
		}
		return results.values, keyErr
	case "select":
		predicates, predicateErr := e.eval(n.child, input)
		results := newResultAccumulator(0)
		for _, predicate := range predicates {
			if truthy(predicate) {
				if err := results.add(input); err != nil {
					return results.values, err
				}
			}
		}
		return results.values, predicateErr
	case "map":
		itemCount := 0
		switch input.kind {
		case valueArray:
			itemCount = len(input.array)
		case valueObject:
			itemCount = len(input.obj.members)
		default:
			return nil, runtimeErrorf("cannot iterate over %s", typeName(input))
		}
		mapped := make([]value, 0)
		nodes, size := 1, 2
		retainedNodes, retainedBytes := 1, 2
		if err := e.retention.retainSize(retainedNodes, retainedBytes); err != nil {
			return nil, err
		}
		defer func() {
			e.retention.releaseSize(retainedNodes, retainedBytes)
		}()
		for i := 0; i < itemCount; i++ {
			item := value{}
			if input.kind == valueArray {
				item = input.array[i]
			} else {
				item = input.obj.members[i].value
			}
			if err := e.tick(); err != nil {
				return nil, err
			}
			outputs, err := e.eval(n.child, item)
			if err != nil {
				return nil, err
			}
			for _, output := range outputs {
				separator := 0
				if len(mapped) > 0 {
					separator = 1
				}
				if err := addAggregate(&nodes, &size, output, separator, MaxValueNodes, MaxValueBytes); err != nil {
					return nil, err
				}
				if err := e.retention.retainSize(output.nodes, output.bytes+separator); err != nil {
					return nil, err
				}
				retainedNodes += output.nodes
				retainedBytes += output.bytes + separator
				mapped = append(mapped, output)
			}
			clear(outputs)
			outputs = nil
		}
		v, err := arrayValue(mapped)
		if err != nil {
			return nil, err
		}
		return []value{v}, nil
	default:
		return nil, runtimeErrorf("function %s is not supported", n.name)
	}
}

func fieldValue(input value, field string) (value, error) {
	switch input.kind {
	case valueNull:
		return null(), nil
	case valueObject:
		v, ok := input.obj.get(field)
		if !ok {
			return null(), nil
		}
		return v, nil
	default:
		return value{}, runtimeErrorf("cannot index %s with string %q", typeName(input), field)
	}
}

func indexValue(input, index value) (value, error) {
	switch input.kind {
	case valueNull:
		if index.kind == valueString || index.kind == valueNumber || index.kind == valueObject {
			return null(), nil
		}
		return value{}, runtimeErrorf("cannot index null with %s", typeName(index))
	case valueObject:
		if index.kind != valueString {
			return value{}, runtimeErrorf("cannot index object with %s", typeName(index))
		}
		v, ok := input.obj.get(index.str)
		if !ok {
			return null(), nil
		}
		return v, nil
	case valueArray:
		if index.kind != valueNumber {
			return value{}, runtimeErrorf("cannot index array with %s", typeName(index))
		}
		i, ok := index.num.indexInt64()
		if !ok {
			return null(), nil
		}
		if i < 0 {
			i += int64(len(input.array))
		}
		if i < 0 || i >= int64(len(input.array)) {
			return null(), nil
		}
		return input.array[i], nil
	default:
		return value{}, runtimeErrorf("cannot index %s", typeName(input))
	}
}

func truthy(v value) bool {
	return v.kind != valueNull && (v.kind != valueBoolean || v.bool)
}

func typeName(v value) string {
	switch v.kind {
	case valueNull:
		return "null"
	case valueBoolean:
		return "boolean"
	case valueNumber:
		return "number"
	case valueString:
		return "string"
	case valueArray:
		return "array"
	case valueObject:
		return "object"
	default:
		return "unknown"
	}
}

func lengthValue(input value) (value, error) {
	switch input.kind {
	case valueNull:
		return int64Value(0), nil
	case valueNumber:
		if input.num.isInteger() {
			return integerValue(new(big.Int).Abs(input.num.integer))
		}
		return floatValue(math.Abs(input.num.float))
	case valueString:
		return int64Value(int64(utf8.RuneCountInString(input.str))), nil
	case valueArray:
		return int64Value(int64(len(input.array))), nil
	case valueObject:
		return int64Value(int64(len(input.obj.members))), nil
	default:
		return value{}, runtimeErrorf("boolean has no length")
	}
}

func keysValue(input value) (value, error) {
	items := make([]value, 0)
	switch input.kind {
	case valueArray:
		items = make([]value, len(input.array))
		for i := range input.array {
			items[i] = int64Value(int64(i))
		}
	case valueObject:
		keys := make([]string, 0, len(input.obj.members))
		for _, member := range input.obj.members {
			keys = append(keys, member.key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			v, _ := stringValue(key)
			items = append(items, v)
		}
	default:
		return value{}, runtimeErrorf("%s has no keys", typeName(input))
	}
	return arrayValue(items)
}

func hasValue(input, key value) (bool, error) {
	switch input.kind {
	case valueNull:
		return false, nil
	case valueObject:
		if key.kind != valueString {
			return false, runtimeErrorf("object keys must be strings")
		}
		_, ok := input.obj.get(key.str)
		return ok, nil
	case valueArray:
		if key.kind != valueNumber {
			return false, runtimeErrorf("array indices must be numbers")
		}
		i, ok := key.num.indexInt64()
		if !ok {
			return false, nil
		}
		return i >= 0 && i < int64(len(input.array)), nil
	default:
		return false, runtimeErrorf("cannot check whether %s has a key", typeName(input))
	}
}

func negateNumber(input value) (value, error) {
	if input.kind != valueNumber {
		return value{}, runtimeErrorf("cannot negate %s", typeName(input))
	}
	if input.num.isInteger() {
		return integerValue(new(big.Int).Neg(input.num.integer))
	}
	return floatValue(-input.num.float)
}

func (e *evaluator) applyBinary(op tokenKind, left, right value) (value, error) {
	if isComparison(op) {
		cmp, err := e.compareValues(left, right)
		if err != nil {
			return value{}, err
		}
		switch op {
		case tokenEqual:
			return boolean(cmp == 0), nil
		case tokenNotEqual:
			return boolean(cmp != 0), nil
		case tokenLess:
			return boolean(cmp < 0), nil
		case tokenLessEqual:
			return boolean(cmp <= 0), nil
		case tokenGreater:
			return boolean(cmp > 0), nil
		case tokenGreaterEqual:
			return boolean(cmp >= 0), nil
		}
	}
	switch op {
	case tokenPlus:
		return addValues(left, right)
	case tokenMinus:
		if left.kind == valueArray && right.kind == valueArray {
			return e.subtractArrays(left, right)
		}
		return numericBinary(op, left, right)
	case tokenMultiply:
		if left.kind == valueString && right.kind == valueNumber {
			return repeatString(left.str, right)
		}
		if right.kind == valueString && left.kind == valueNumber {
			return repeatString(right.str, left)
		}
		if left.kind == valueObject && right.kind == valueObject {
			return e.mergeObjectsRecursive(left, right)
		}
		return numericBinary(op, left, right)
	case tokenDivide:
		if left.kind == valueString && right.kind == valueString {
			if left.str == "" {
				return arrayValue(nil)
			}
			partCount := 1
			if right.str == "" {
				partCount = utf8.RuneCountInString(left.str)
			} else {
				partCount += strings.Count(left.str, right.str)
			}
			if partCount > MaxValueNodes-1 {
				return value{}, errValueNodes
			}
			parts := strings.Split(left.str, right.str)
			items := make([]value, 0, len(parts))
			for _, part := range parts {
				item, err := stringValue(part)
				if err != nil {
					return value{}, err
				}
				items = append(items, item)
			}
			return arrayValue(items)
		}
		return numericBinary(op, left, right)
	case tokenModulo:
		return numericBinary(op, left, right)
	default:
		return value{}, errors.New("unsupported binary operator")
	}
}

func addValues(left, right value) (value, error) {
	if left.kind == valueNull {
		return right, nil
	}
	if right.kind == valueNull {
		return left, nil
	}
	switch {
	case left.kind == valueNumber && right.kind == valueNumber:
		return numericBinary(tokenPlus, left, right)
	case left.kind == valueString && right.kind == valueString:
		if len(left.str) > MaxValueBytes-len(right.str) {
			return value{}, errValueBytes
		}
		return stringValue(left.str + right.str)
	case left.kind == valueArray && right.kind == valueArray:
		items := make([]value, 0, len(left.array)+len(right.array))
		items = append(items, left.array...)
		items = append(items, right.array...)
		return arrayValue(items)
	case left.kind == valueObject && right.kind == valueObject:
		items := make([]objectMember, 0, len(left.obj.members)+len(right.obj.members))
		items = append(items, left.obj.members...)
		items = append(items, right.obj.members...)
		return object(items)
	default:
		return value{}, runtimeErrorf("cannot add %s and %s", typeName(left), typeName(right))
	}
}

func numericBinary(op tokenKind, left, right value) (value, error) {
	if left.kind != valueNumber || right.kind != valueNumber {
		return value{}, runtimeErrorf("%s and %s cannot be used in arithmetic", typeName(left), typeName(right))
	}
	if left.num.isInteger() && right.num.isInteger() {
		a, b := left.num.integer, right.num.integer
		switch op {
		case tokenPlus:
			return integerValue(new(big.Int).Add(a, b))
		case tokenMinus:
			return integerValue(new(big.Int).Sub(a, b))
		case tokenMultiply:
			return integerValue(new(big.Int).Mul(a, b))
		case tokenDivide:
			if b.Sign() == 0 {
				return value{}, runtimeErrorf("cannot divide by zero")
			}
			quotient, remainder := new(big.Int), new(big.Int)
			quotient.QuoRem(a, b, remainder)
			if remainder.Sign() == 0 {
				return integerValue(quotient)
			}
			leftFloat, _ := new(big.Float).SetInt(a).Float64()
			rightFloat, _ := new(big.Float).SetInt(b).Float64()
			return floatValue(leftFloat / rightFloat)
		case tokenModulo:
			if b.Sign() == 0 {
				return value{}, runtimeErrorf("cannot calculate remainder with zero")
			}
			return integerValue(new(big.Int).Rem(a, b))
		}
	}

	a := numberFloat(left.num)
	b := numberFloat(right.num)
	switch op {
	case tokenPlus:
		return floatValue(a + b)
	case tokenMinus:
		return floatValue(a - b)
	case tokenMultiply:
		return floatValue(a * b)
	case tokenDivide:
		if b == 0 {
			return value{}, runtimeErrorf("cannot divide by zero")
		}
		return floatValue(a / b)
	case tokenModulo:
		a, b = math.Trunc(a), math.Trunc(b)
		if b == 0 {
			return value{}, runtimeErrorf("cannot calculate remainder with zero")
		}
		return floatValue(math.Mod(a, b))
	default:
		return value{}, errors.New("unsupported numeric operator")
	}
}

func numberFloat(n number) float64 {
	if !n.isInteger() {
		return n.float
	}
	f, _ := new(big.Float).SetInt(n.integer).Float64()
	return f
}

func repeatString(s string, count value) (value, error) {
	if count.num.isInteger() {
		if count.num.integer.Sign() < 0 {
			return null(), nil
		}
	} else if count.num.float < 0 {
		return null(), nil
	}
	if len(s) == 0 {
		return stringValue("")
	}

	var n int64
	if count.num.isInteger() {
		if !count.num.integer.IsInt64() {
			return value{}, runtimeErrorf("string repetition count is out of range")
		}
		n = count.num.integer.Int64()
	} else {
		truncated := math.Trunc(count.num.float)
		if truncated > math.MaxInt64 || truncated < math.MinInt64 {
			return value{}, runtimeErrorf("string repetition count is out of range")
		}
		n = int64(truncated)
	}
	if n == 0 {
		return stringValue("")
	}
	if n > int64(MaxValueBytes) || int64(len(s)) > int64(MaxValueBytes)/n {
		return value{}, errValueBytes
	}
	return stringValue(strings.Repeat(s, int(n)))
}

func (e *evaluator) subtractArrays(left, right value) (value, error) {
	items := make([]value, 0, len(left.array))
	for _, candidate := range left.array {
		remove := false
		for _, excluded := range right.array {
			if err := e.tick(); err != nil {
				return value{}, err
			}
			equal, err := e.valuesEqual(candidate, excluded)
			if err != nil {
				return value{}, err
			}
			if equal {
				remove = true
				break
			}
		}
		if !remove {
			items = append(items, candidate)
		}
	}
	return arrayValue(items)
}

func (e *evaluator) mergeObjectsRecursive(left, right value) (value, error) {
	items := make([]objectMember, 0, len(left.obj.members)+len(right.obj.members))
	items = append(items, left.obj.members...)
	for _, rightMember := range right.obj.members {
		if err := e.tick(); err != nil {
			return value{}, err
		}
		mergedValue := rightMember.value
		if leftValue, ok := left.obj.get(rightMember.key); ok && leftValue.kind == valueObject && rightMember.value.kind == valueObject {
			var err error
			mergedValue, err = e.mergeObjectsRecursive(leftValue, rightMember.value)
			if err != nil {
				return value{}, err
			}
		}
		items = append(items, objectMember{key: rightMember.key, value: mergedValue})
	}
	return object(items)
}

func (e *evaluator) valuesEqual(left, right value) (bool, error) {
	if err := e.tick(); err != nil {
		return false, err
	}
	if left.kind == valueNumber && right.kind == valueNumber {
		return compareNumbers(left.num, right.num) == 0, nil
	}
	if left.kind != right.kind {
		return false, nil
	}
	switch left.kind {
	case valueNull:
		return true, nil
	case valueBoolean:
		return left.bool == right.bool, nil
	case valueString:
		return left.str == right.str, nil
	case valueArray:
		if len(left.array) != len(right.array) {
			return false, nil
		}
		for i := range left.array {
			equal, err := e.valuesEqual(left.array[i], right.array[i])
			if err != nil || !equal {
				return equal, err
			}
		}
		return true, nil
	case valueObject:
		if len(left.obj.members) != len(right.obj.members) {
			return false, nil
		}
		for _, member := range left.obj.members {
			rightValue, ok := right.obj.get(member.key)
			if !ok {
				return false, nil
			}
			equal, err := e.valuesEqual(member.value, rightValue)
			if err != nil || !equal {
				return equal, err
			}
		}
		return true, nil
	default:
		return false, nil
	}
}

func (e *evaluator) compareValues(left, right value) (int, error) {
	if err := e.tick(); err != nil {
		return 0, err
	}
	if left.kind != right.kind {
		return compareInt(valueRank(left), valueRank(right)), nil
	}
	switch left.kind {
	case valueNull:
		return 0, nil
	case valueBoolean:
		return compareInt(boolInt(left.bool), boolInt(right.bool)), nil
	case valueNumber:
		return compareNumbers(left.num, right.num), nil
	case valueString:
		return strings.Compare(left.str, right.str), nil
	case valueArray:
		for i := 0; i < len(left.array) && i < len(right.array); i++ {
			cmp, err := e.compareValues(left.array[i], right.array[i])
			if err != nil || cmp != 0 {
				return cmp, err
			}
		}
		return compareInt(len(left.array), len(right.array)), nil
	case valueObject:
		leftKeys, err := e.sortedObjectKeys(left.obj)
		if err != nil {
			return 0, err
		}
		rightKeys, err := e.sortedObjectKeys(right.obj)
		if err != nil {
			return 0, err
		}
		for i := 0; i < len(leftKeys) && i < len(rightKeys); i++ {
			if cmp := strings.Compare(leftKeys[i], rightKeys[i]); cmp != 0 {
				return cmp, nil
			}
		}
		if cmp := compareInt(len(leftKeys), len(rightKeys)); cmp != 0 {
			return cmp, nil
		}
		for i := range leftKeys {
			leftValue, _ := left.obj.get(leftKeys[i])
			rightValue, _ := right.obj.get(rightKeys[i])
			cmp, err := e.compareValues(leftValue, rightValue)
			if err != nil || cmp != 0 {
				return cmp, err
			}
		}
		return 0, nil
	default:
		return 0, nil
	}
}

func compareNumbers(left, right number) int {
	if left.isInteger() && right.isInteger() {
		return left.integer.Cmp(right.integer)
	}
	leftRat, rightRat := new(big.Rat), new(big.Rat)
	if left.isInteger() {
		leftRat.SetInt(left.integer)
	} else {
		leftRat.SetFloat64(left.float)
	}
	if right.isInteger() {
		rightRat.SetInt(right.integer)
	} else {
		rightRat.SetFloat64(right.float)
	}
	return leftRat.Cmp(rightRat)
}

func valueRank(v value) int {
	switch v.kind {
	case valueNull:
		return 0
	case valueBoolean:
		if v.bool {
			return 2
		}
		return 1
	case valueNumber:
		return 3
	case valueString:
		return 4
	case valueArray:
		return 5
	case valueObject:
		return 6
	default:
		return 7
	}
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func compareInt(left, right int) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func (e *evaluator) sortedObjectKeys(object objectValue) ([]string, error) {
	keys := make([]string, 0, len(object.members))
	for _, member := range object.members {
		if err := e.tick(); err != nil {
			return nil, err
		}
		keys = append(keys, member.key)
	}
	sort.Strings(keys)
	return keys, nil
}
