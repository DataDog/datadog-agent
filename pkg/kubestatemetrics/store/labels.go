// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package store

import (
	"errors"
	"iter"
	"maps"
	"slices"
)

// Labels are the labels of a metric, as parallel slices of keys and values.
//
// The store keeps the labels of every series of the cluster for the lifetime
// of the objects. A map per series costs several hundred bytes, while the keys
// of a family are usually the same for all its series, so they are shared and
// only the values are kept per series.
//
// The slices must not be modified: the keys are shared between series, and
// both slices may come from the metric generators of kube-state-metrics.
type Labels struct {
	keys   []string
	values []string
}

// errLabelsLength is returned when the keys and the values of labels have
// different lengths.
var errLabelsLength = errors.New("LabelKeys and LabelValues not same size")

// NewLabels returns the labels made of keys and values, which must have the
// same length. When a key is repeated, its last value wins, as in a map.
func NewLabels(keys, values []string) (Labels, error) {
	if len(keys) != len(values) {
		return Labels{}, errLabelsLength
	}
	if hasDuplicateKeys(keys) {
		return dedupLabels(keys, values), nil
	}
	return Labels{keys: keys, values: values}, nil
}

// LabelsFromMap returns the labels of m, sorted by key.
func LabelsFromMap(m map[string]string) Labels {
	keys := slices.Sorted(maps.Keys(m))
	values := make([]string, len(keys))
	for i, key := range keys {
		values[i] = m[key]
	}
	return Labels{keys: keys, values: values}
}

// Len returns the number of labels.
func (l Labels) Len() int {
	return len(l.keys)
}

// Lookup returns the value of the label key, and whether it is set.
func (l Labels) Lookup(key string) (string, bool) {
	for i, k := range l.keys {
		if k == key {
			return l.values[i], true
		}
	}
	return "", false
}

// Get returns the value of the label key, or an empty string when it is not
// set.
func (l Labels) Get(key string) string {
	value, _ := l.Lookup(key)
	return value
}

// All iterates over the labels, in the order of their keys.
func (l Labels) All() iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		for i, key := range l.keys {
			if !yield(key, l.values[i]) {
				return
			}
		}
	}
}

// maxKeysForQuadraticDedup is the number of keys up to which comparing every
// pair of keys is cheaper than building a set: comparing different keys mostly
// stops at their lengths, while a set allocates.
const maxKeysForQuadraticDedup = 64

func hasDuplicateKeys(keys []string) bool {
	if len(keys) <= maxKeysForQuadraticDedup {
		for i := 1; i < len(keys); i++ {
			if slices.Contains(keys[:i], keys[i]) {
				return true
			}
		}
		return false
	}
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if _, found := seen[key]; found {
			return true
		}
		seen[key] = struct{}{}
	}
	return false
}

// dedupLabels returns new labels where each key appears once, with its last
// value, at the position of its first occurrence.
func dedupLabels(keys, values []string) Labels {
	index := make(map[string]int, len(keys))
	l := Labels{
		keys:   make([]string, 0, len(keys)),
		values: make([]string, 0, len(keys)),
	}
	for i, key := range keys {
		if j, found := index[key]; found {
			l.values[j] = values[i]
			continue
		}
		index[key] = len(l.keys)
		l.keys = append(l.keys, key)
		l.values = append(l.values, values[i])
	}
	return l
}

// keysInterner shares the keys of the series of a family between the series:
// the keys of a family are usually the same for all its series. The families
// of labels and annotations have different keys per object, but the objects of
// a workload usually share theirs, so it keeps the last few sets of keys of
// each family, which bounds its size by the number of families of the store.
type keysInterner map[string]*recentKeys

// maxRecentKeys is the number of sets of keys kept per family.
const maxRecentKeys = 8

type recentKeys struct {
	keys [maxRecentKeys][]string
	// next is where the next new set of keys replaces the oldest one.
	next int
}

// intern returns labels equal to l, whose keys can have duplicates, as
// NewLabels would. Their keys are shared with the previous series of the family
// when one of the last sets of keys of the family is equal. Otherwise, they are
// a copy of the keys of l, which doesn't keep the block kube-state-metrics
// allocated them in.
//
// Only the new sets of keys are checked for duplicates: the shared ones have
// none, and a set with duplicates never equals one of them.
func (ki keysInterner) intern(family string, l Labels) Labels {
	if l.keys == nil {
		return l
	}
	recent, found := ki[family]
	if !found {
		recent = &recentKeys{}
		ki[family] = recent
	}
	for _, shared := range recent.keys {
		if shared != nil && slices.Equal(shared, l.keys) {
			l.keys = shared
			return l
		}
	}
	if hasDuplicateKeys(l.keys) {
		// Rare: not shared, so that the set with duplicates is checked again
		return dedupLabels(l.keys, l.values)
	}
	l.keys = slices.Clone(l.keys)
	recent.keys[recent.next] = l.keys
	recent.next = (recent.next + 1) % maxRecentKeys
	return l
}
