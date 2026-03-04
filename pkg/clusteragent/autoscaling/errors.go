// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoscaling

import (
	"fmt"
)

// ConditionReasonType is a typed string for programmatic condition reasons.
// Values must be CamelCase with no spaces, following the Kubernetes convention.
type ConditionReasonType string

const (
	// ConditionReasonInvalidTargetRef indicates the targetRef API version could not be parsed.
	ConditionReasonInvalidTargetRef ConditionReasonType = "InvalidTargetRef"
	// ConditionReasonInvalidTarget indicates the target cannot be autoscaled (e.g. it is the cluster agent itself).
	ConditionReasonInvalidTarget ConditionReasonType = "InvalidTarget"
	// ConditionReasonInvalidSpec indicates the DPA spec failed validation (e.g. bad objectives or constraints).
	ConditionReasonInvalidSpec ConditionReasonType = "InvalidSpec"
	// ConditionReasonClusterAutoscalerLimitReached indicates the maximum number of DPA objects per cluster has been reached.
	ConditionReasonClusterAutoscalerLimitReached ConditionReasonType = "ClusterAutoscalerLimitReached"
	// ConditionReasonRecommendationError indicates Datadog could not compute recommendations.
	ConditionReasonRecommendationError ConditionReasonType = "RecommendationError"
	// ConditionReasonLocalRecommenderError indicates the local fallback recommender could not compute recommendations.
	ConditionReasonLocalRecommenderError ConditionReasonType = "LocalRecommenderError"
	// ConditionReasonTargetNotFound indicates the scale target could not be found.
	ConditionReasonTargetNotFound ConditionReasonType = "TargetNotFound"
	// ConditionReasonScaleFailed indicates the scale operation on the target failed.
	ConditionReasonScaleFailed ConditionReasonType = "ScaleFailed"
	// ConditionReasonScalingDisabled indicates scaling is blocked because current replicas is 0.
	ConditionReasonScalingDisabled ConditionReasonType = "ScalingDisabled"
	// ConditionReasonPolicyRestricted indicates the applyPolicy or strategy blocks the scaling action.
	ConditionReasonPolicyRestricted ConditionReasonType = "PolicyRestricted"
	// ConditionReasonFallbackRestricted indicates the fallback scaling direction is not allowed.
	ConditionReasonFallbackRestricted ConditionReasonType = "FallbackRestricted"
	// ConditionReasonUnsupportedTargetKind indicates vertical rollout is not supported for this workload Kind.
	ConditionReasonUnsupportedTargetKind ConditionReasonType = "UnsupportedTargetKind"
	// ConditionReasonRolloutFailed indicates a failure when triggering a vertical rollout on the target.
	ConditionReasonRolloutFailed ConditionReasonType = "RolloutFailed"
	// ConditionReasonLimitedByConstraint indicates the recommendation was clamped by configured constraints
	// (e.g. min/max replicas for horizontal, minAllowed/maxAllowed for vertical).
	ConditionReasonLimitedByConstraint ConditionReasonType = "LimitedByConstraint"
	// ConditionReasonLimitedByScalingBehavior indicates scaling was limited by behavior settings
	// (e.g. stabilization window, scaling rules).
	ConditionReasonLimitedByScalingBehavior ConditionReasonType = "LimitedByScalingBehavior"
)

// ConditionReason is an interface that errors can implement to provide
// a programmatic Reason for Kubernetes conditions.
// When an error implements this interface, Reason() populates the condition's
// Reason field and Error() populates the Message field.
// When an error does not implement this interface, Error() populates the
// Message field and Reason is left empty.
type ConditionReason interface {
	Reason() ConditionReasonType
}

// NewConditionError wraps an existing error with a programmatic reason.
func NewConditionError(reason ConditionReasonType, err error) error {
	return &conditionError{reason: reason, err: err}
}

// NewConditionErrorf creates a new error with a programmatic reason and a formatted message.
func NewConditionErrorf(reason ConditionReasonType, format string, args ...any) error {
	return &conditionError{reason: reason, err: fmt.Errorf(format, args...)}
}

type conditionError struct {
	reason ConditionReasonType
	err    error
}

func (e *conditionError) Error() string               { return e.err.Error() }
func (e *conditionError) Unwrap() error               { return e.err }
func (e *conditionError) Reason() ConditionReasonType { return e.reason }
