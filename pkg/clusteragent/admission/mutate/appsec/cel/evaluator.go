// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && cel

// Package cel provides CEL expression evaluation utilities for AppSec pod matching
// using the workloadfilter component's CEL infrastructure.
package cel

import (
	"fmt"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/celprogram"
	corev1 "k8s.io/api/core/v1"
)

// PodEvaluator evaluates CEL expressions against Kubernetes pods using workloadfilter's CEL infrastructure
type PodEvaluator struct {
	programs map[string]*program.CELProgram
}

// NewPodEvaluator creates a new CEL evaluator for pod matching
func NewPodEvaluator() *PodEvaluator {
	return &PodEvaluator{
		programs: make(map[string]*program.CELProgram),
	}
}

// Matches evaluates a CEL expression against a pod and returns true if it matches
// The expression should use "pod" as the root variable to reference the pod.
// Example: "'app' in pod.labels && pod.labels['app'] == 'myapp'"
func (e *PodEvaluator) Matches(expression string, pod *corev1.Pod) (bool, error) {
	// Get or create the CEL program for this expression
	prog, err := e.getOrCreateProgram(expression)
	if err != nil {
		return false, err
	}

	// Convert k8s pod to workloadfilter filterable pod
	filterablePod := workloadfilter.CreatePod(
		string(pod.UID),
		pod.Name,
		pod.Namespace,
		pod.Annotations,
		pod.Labels,
	)

	// Evaluate the program - it returns Excluded if the expression evaluates to true
	// We invert this since we're using it for matching (inclusion) not exclusion
	result := prog.Evaluate(filterablePod)

	// The CEL program in workloadfilter returns Excluded when expression is true
	// We want to return true when the pod matches, so we check for Excluded
	return result == workloadfilter.Excluded, nil
}

// getOrCreateProgram gets a cached CEL program or creates a new one
func (e *PodEvaluator) getOrCreateProgram(expression string) (*program.CELProgram, error) {
	if prog, ok := e.programs[expression]; ok {
		return prog, nil
	}

	// Create a new CEL program using workloadfilter utilities
	celProg, err := celprogram.CreateCELProgram(expression, workloadfilter.PodType)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL program: %w", err)
	}

	prog := &program.CELProgram{
		Name:    "appsec-pod-matcher",
		Exclude: celProg,
	}

	// Check for initialization errors
	if errs := prog.GetInitializationErrors(); len(errs) > 0 {
		return nil, fmt.Errorf("CEL program initialization errors: %v", errs)
	}

	// Cache the program
	e.programs[expression] = prog

	return prog, nil
}

// CompiledMatcher is a pre-compiled CEL expression for efficient repeated evaluation
type CompiledMatcher struct {
	expression string
	program    *program.CELProgram
}

// Compile pre-compiles a CEL expression for efficient repeated evaluation
func (e *PodEvaluator) Compile(expression string) (*CompiledMatcher, error) {
	prog, err := e.getOrCreateProgram(expression)
	if err != nil {
		return nil, err
	}

	return &CompiledMatcher{
		expression: expression,
		program:    prog,
	}, nil
}

// Matches evaluates the compiled expression against a pod
func (m *CompiledMatcher) Matches(pod *corev1.Pod) (bool, error) {
	filterablePod := workloadfilter.CreatePod(
		string(pod.UID),
		pod.Name,
		pod.Namespace,
		pod.Annotations,
		pod.Labels,
	)
	result := m.program.Evaluate(filterablePod)
	return result == workloadfilter.Excluded, nil
}

// Expression returns the original CEL expression string
func (m *CompiledMatcher) Expression() string {
	return m.expression
}
