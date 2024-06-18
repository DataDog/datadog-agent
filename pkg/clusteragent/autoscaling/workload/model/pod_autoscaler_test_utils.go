// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package model

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"

	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
)

type FakePodAutoscalerInternal struct {
	Namespace                 string
	Name                      string
	Generation                int64
	Spec                      *datadoghq.DatadogPodAutoscalerSpec
	SettingsTimestamp         time.Time
	ScalingValues             ScalingValues
	HorizontalLastAction      *datadoghq.DatadogPodAutoscalerHorizontalAction
	HorizontalLastActionError error
	VerticalLastAction        *datadoghq.DatadogPodAutoscalerVerticalAction
	VerticalLastActionError   error
	CurrentReplicas           *int32
	ScaledReplicas            *int32
	Error                     error
	Deleted                   bool
	TargetGVK                 schema.GroupVersionKind
}

func (f FakePodAutoscalerInternal) Build() PodAutoscalerInternal {
	return PodAutoscalerInternal{
		namespace:                 f.Namespace,
		name:                      f.Name,
		generation:                f.Generation,
		spec:                      f.Spec,
		settingsTimestamp:         f.SettingsTimestamp,
		scalingValues:             f.ScalingValues,
		horizontalLastAction:      f.HorizontalLastAction,
		horizontalLastActionError: f.HorizontalLastActionError,
		verticalLastAction:        f.VerticalLastAction,
		verticalLastActionError:   f.VerticalLastActionError,
		currentReplicas:           f.CurrentReplicas,
		scaledReplicas:            f.ScaledReplicas,
		error:                     f.Error,
		deleted:                   f.Deleted,
		targetGVK:                 f.TargetGVK,
	}
}

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
		cmpopts.SortSlices(func(a, b PodAutoscalerInternal) bool {
			return a.ID() < b.ID()
		}),
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
				if real, ok := x.(PodAutoscalerInternal); ok {
					return real
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
				if real, ok := x.([]PodAutoscalerInternal); ok {
					return real
				}
				if fake, ok := x.([]FakePodAutoscalerInternal); ok {
					var result []PodAutoscalerInternal
					for _, f := range fake {
						result = append(result, f.Build())
					}
					return result
				}
				panic("filer failed - unexpected type")
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
