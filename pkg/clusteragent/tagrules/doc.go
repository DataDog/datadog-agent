// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package tagrules implements the Datadog Cluster Agent tag-rule controller.
//
// It reconciles TagRule CRs (agent.datadoghq.com/v1alpha1) by writing custom tags
// onto matching Kubernetes entities, so that the node agents' tagger and
// host-tags flows pick them up and apply them to metrics:
//
//	pods:  tags are merged into the per-container JSON annotation
//	       ad.datadoghq.com/<container>.tags (unconditional tagger pickup).
//	       Keys owned by rules are recorded on the pod in the ledger
//	       annotation datadoghq.com/managed-tag-keys, so user-set keys are
//	       never touched and stale owned keys can be stripped.
//	nodes: tags are written as annotations under the reserved prefix
//	       tags.datadoghq.com/tag.<key> (unconditional pickup in the
//	       host-tags flow and the DCA tagger).
//
// Rule values are restricted to bools and declared string sets: the value
// domain is the cardinality contract. Source-object comparison (for example a
// leader-election Lease) is supported through CEL expressions over `entity`
// and `source`. The controller evaluates declaratively: it recomputes the full
// desired owned-key set from the current rules and cluster state on every
// reconcile, writes at most one patch per entity per pass, and never
// accumulates. Controller wins on drift.
//
// Lifecycle: the controller adds the datadoghq.com/tag-rule-cleanup finalizer;
// on rule deletion it sweeps entities matching the last-observed selector
// (reconciling them with the rule removed, which strips owned keys and updates
// the ledger) before removing the finalizer.

package tagrules
