// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
)

// Mutator provides a common interface for building components capable of mutating pods so that individual webhooks can
// share mutators.
type Mutator interface {
	// MutatePod will optionally mutate a pod, returning true if mutation occurs and an error if there is a problem.
	MutatePod(pod *corev1.Pod, ns string, dc dynamic.Interface) (bool, error)
}

// MutatorWithFilter is a combination interface of Mutator and MutationFilter such that a single struct can be used to
// both mutate a pod and determine if a pod should be mutated.
type MutatorWithFilter interface {
	Mutator
	MutationFilter
}

// MutatorFunc is a function that mutates a pod
type MutatorFunc func(pod *corev1.Pod, ns string, cl dynamic.Interface) (bool, error)

// MutatePod allows MutatorFunc to satisfy the Mutator interface.
func (f MutatorFunc) MutatePod(pod *corev1.Pod, ns string, dc dynamic.Interface) (bool, error) {
	return f(pod, ns, dc)
}

// Mutators is a Mutator that combines multiple Mutators into a single Mutator.
type Mutators struct {
	mutators []Mutator
}

// NewMutators creates a new mutator with the provided Mutators.
func NewMutators(mutators ...Mutator) *Mutators {
	return &Mutators{
		mutators: mutators,
	}
}

// MutatePod will call MutatePod on each Mutator, returning true if any Mutator mutates the pod and an error if there is
// a problem.
func (m *Mutators) MutatePod(pod *corev1.Pod, ns string, dc dynamic.Interface) (bool, error) {
	mutated := false
	for _, mutator := range m.mutators {
		mutatedPod, err := mutator.MutatePod(pod, ns, dc)
		if err != nil {
			return mutated, err
		}
		mutated = mutated || mutatedPod
	}
	return mutated, nil
}
