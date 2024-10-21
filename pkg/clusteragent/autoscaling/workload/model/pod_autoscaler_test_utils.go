// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package model

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"

	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
)

// FakePodAutoscalerInternal is a fake PodAutoscalerInternal object.
type FakePodAutoscalerInternal struct {
	Namespace                 string
	Name                      string
	Generation                int64
	Spec                      *datadoghq.DatadogPodAutoscalerSpec
	SettingsTimestamp         time.Time
	CreationTimestamp         time.Time
	ScalingValues             ScalingValues
	HorizontalLastActions     []datadoghq.DatadogPodAutoscalerHorizontalAction
	HorizontalLastLimitReason string
	HorizontalLastActionError error
	HorizontalEventsRetention time.Duration
	VerticalLastAction        *datadoghq.DatadogPodAutoscalerVerticalAction
	VerticalLastActionError   error
	CurrentReplicas           *int32
	ScaledReplicas            *int32
	Error                     error
	Deleted                   bool
	TargetGVK                 schema.GroupVersionKind
}

// Build creates a PodAutoscalerInternal object from the FakePodAutoscalerInternal.
func (f FakePodAutoscalerInternal) Build() PodAutoscalerInternal {
	return PodAutoscalerInternal{
		namespace:                 f.Namespace,
		name:                      f.Name,
		generation:                f.Generation,
		spec:                      f.Spec,
		settingsTimestamp:         f.SettingsTimestamp,
		creationTimestamp:         f.CreationTimestamp,
		scalingValues:             f.ScalingValues,
		horizontalLastActions:     f.HorizontalLastActions,
		horizontalLastLimitReason: f.HorizontalLastLimitReason,
		horizontalLastActionError: f.HorizontalLastActionError,
		horizontalEventsRetention: f.HorizontalEventsRetention,
		verticalLastAction:        f.VerticalLastAction,
		verticalLastActionError:   f.VerticalLastActionError,
		currentReplicas:           f.CurrentReplicas,
		scaledReplicas:            f.ScaledReplicas,
		error:                     f.Error,
		deleted:                   f.Deleted,
		targetGVK:                 f.TargetGVK,
	}
}

// AddHorizontalAction mimics the behavior of adding an horizontal event.
func (f *FakePodAutoscalerInternal) AddHorizontalAction(currentTime time.Time, action *datadoghq.DatadogPodAutoscalerHorizontalAction) {
	f.HorizontalLastActions = addHorizontalAction(currentTime, f.HorizontalEventsRetention, f.HorizontalLastActions, action)
}

// NewFakePodAutoscalerInternal creates a new FakePodAutoscalerInternal object.
func NewFakePodAutoscalerInternal(ns, name string, fake *FakePodAutoscalerInternal) PodAutoscalerInternal {
	if fake == nil {
		fake = &FakePodAutoscalerInternal{}
	}

	fake.Namespace = ns
	fake.Name = name
	return fake.Build()
}

// ComparePodAutoscalers compares two PodAutoscalerInternal objects with cmp.Diff.
func ComparePodAutoscalers(expected any, actual any) string {
	return cmp.Diff(
		expected, actual,
		cmpopts.EquateErrors(),
		cmp.Exporter(func(t reflect.Type) bool {
			return t == reflect.TypeOf(PodAutoscalerInternal{})
		}),
		cmp.FilterValues(
			func(x, y interface{}) bool {
				for _, v := range []interface{}{x, y} {
					switch v.(type) {
					case FakePodAutoscalerInternal:
					case PodAutoscalerInternal:
						return true
					}
				}
				return false
			},
			cmp.Transformer("model.FakeToInternal", func(x any) PodAutoscalerInternal {
				if actual, ok := x.(PodAutoscalerInternal); ok {
					return actual
				}
				if fake, ok := x.(FakePodAutoscalerInternal); ok {
					return fake.Build()
				}
				panic("filer failed - unexpected type")
			}),
		),
		cmp.FilterValues(
			func(x, y interface{}) bool {
				for _, v := range []interface{}{x, y} {
					switch v.(type) {
					case []FakePodAutoscalerInternal:
					case []PodAutoscalerInternal:
						return true
					}
				}
				return false
			},
			cmp.Transformer("model.FakeToInternalList", func(x any) []PodAutoscalerInternal {
				var autoscalers []PodAutoscalerInternal
				if actual, ok := x.([]PodAutoscalerInternal); ok {
					autoscalers = actual
				} else if fake, ok := x.([]FakePodAutoscalerInternal); ok {
					for _, f := range fake {
						autoscalers = append(autoscalers, f.Build())
					}
				} else {
					panic("filter on  failed - unexpected type")
				}

				slices.SortStableFunc(autoscalers, func(a, b PodAutoscalerInternal) int {
					return strings.Compare(a.ID(), b.ID())
				})
				return autoscalers
			}),
		),
	)
}

// AssertPodAutoscalersEqual asserts that two PodAutoscalerInternal objects are equal.
func AssertPodAutoscalersEqual(t *testing.T, expected any, actual any) {
	t.Helper()

	diff := ComparePodAutoscalers(expected, actual)
	assert.Empty(t, diff, "## + is content from actual, ## - is content from expected")
}
