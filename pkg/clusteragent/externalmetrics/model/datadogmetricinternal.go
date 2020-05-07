// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package model

import (
	"errors"
	"fmt"
	"time"

	datadoghq "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

const (
	DatadogMetricErrorConditionReason string = "Unable to fetch data from Datadog"
)

// DatadogMetricInternal is a flatten, easier to use, representation of `DatadogMetric` CRD
type DatadogMetricInternal struct {
	ID                 string
	Query              string
	Valid              bool
	Active             bool
	Deleted            bool
	Autogen            bool
	ExternalMetricName string
	Value              float64
	UpdateTime         time.Time
	Error              error
}

// NewDatadogMetricInternal returns a `DatadogMetricInternal` object from a `DatadogMetric` CRD Object
// `id` is expected to be unique and should correspond to `namespace/name`
func NewDatadogMetricInternal(id string, datadogMetric datadoghq.DatadogMetric) DatadogMetricInternal {
	internal := DatadogMetricInternal{
		ID:      id,
		Query:   datadogMetric.Spec.Query,
		Value:   datadogMetric.Status.Value,
		Valid:   false,
		Active:  false,
		Deleted: false,
		Autogen: false,
	}

	if len(datadogMetric.Spec.ExternalMetricName) > 0 {
		internal.Autogen = true
		internal.ExternalMetricName = datadogMetric.Spec.ExternalMetricName
	}

	for _, condition := range datadogMetric.Status.Conditions {
		switch {
		case condition.Type == datadoghq.DatadogMetricConditionTypeValid && condition.Status == corev1.ConditionTrue:
			internal.Valid = true
		case condition.Type == datadoghq.DatadogMetricConditionTypeActive && condition.Status == corev1.ConditionTrue:
			internal.Active = true
		case condition.Type == datadoghq.DatadogMetricConditionTypeUpdated && condition.Status == corev1.ConditionTrue:
			internal.UpdateTime = condition.LastUpdateTime.UTC()
		case condition.Type == datadoghq.DatadogMetricConditionTypeError && condition.Status == corev1.ConditionTrue:
			internal.Error = errors.New(condition.Message)
		}
	}

	// If UpdateTime is not set, it means it's a newly created DatadogMetric
	// We'll need a proper update time to generate status, so setting to current time
	if internal.UpdateTime.IsZero() {
		internal.UpdateTime = time.Now().UTC()
	}

	return internal
}

// NewDatadogMetricInternalFromExternalMetric returns a `DatadogMetricInternal` object
// that is auto-generated from a standard ExternalMetric query (non-DatadogMetric reference)
func NewDatadogMetricInternalFromExternalMetric(id, query, metricName string) DatadogMetricInternal {
	return DatadogMetricInternal{
		ID:                 id,
		Query:              query,
		Valid:              false,
		Active:             true,
		Deleted:            false,
		Autogen:            true,
		ExternalMetricName: metricName,
		UpdateTime:         time.Now().UTC(),
	}
}

// UpdateFrom updates the `DatadogMetricInternal` from `DatadogMetric` Spec, returns modified instance
func (d *DatadogMetricInternal) UpdateFrom(currentSpec datadoghq.DatadogMetricSpec) {
	d.Query = currentSpec.Query
}

// IsNewerThan returns true if the current `DatadogMetricInternal` has been updated more recently than `DatadogMetric` Status
func (d *DatadogMetricInternal) IsNewerThan(currentStatus datadoghq.DatadogMetricStatus) bool {
	for _, condition := range currentStatus.Conditions {
		if condition.Type == datadoghq.DatadogMetricConditionTypeUpdated {
			if condition.Status == corev1.ConditionTrue && condition.LastUpdateTime.UTC().Unix() >= d.UpdateTime.UTC().Unix() {
				return false
			}
			break
		}
	}

	return true
}

// HasBeenUpdated returns true if the current `DatadogMetricInternal` has been update between Now() and Now() - duration
func (d *DatadogMetricInternal) HasBeenUpdatedFor(duration time.Duration) bool {
	return d.UpdateTime.After(time.Now().UTC().Add(-duration))
}

// BuildStatus generates a new status for `DatadogMetric` based on current status and information from `DatadogMetricInternal`
func (d *DatadogMetricInternal) BuildStatus(currentStatus *datadoghq.DatadogMetricStatus) *datadoghq.DatadogMetricStatus {
	updateTime := metav1.NewTime(d.UpdateTime)
	existingConditions := map[datadoghq.DatadogMetricConditionType]*datadoghq.DatadogMetricCondition{
		datadoghq.DatadogMetricConditionTypeActive:  nil,
		datadoghq.DatadogMetricConditionTypeValid:   nil,
		datadoghq.DatadogMetricConditionTypeUpdated: nil,
		datadoghq.DatadogMetricConditionTypeError:   nil,
	}

	if currentStatus != nil {
		for i := range currentStatus.Conditions {
			condition := &currentStatus.Conditions[i]
			if _, ok := existingConditions[condition.Type]; ok {
				existingConditions[condition.Type] = condition
			}
		}
	}

	activeCondition := d.newCondition(d.Active, updateTime, datadoghq.DatadogMetricConditionTypeActive, existingConditions[datadoghq.DatadogMetricConditionTypeActive])
	validCondition := d.newCondition(d.Valid, updateTime, datadoghq.DatadogMetricConditionTypeValid, existingConditions[datadoghq.DatadogMetricConditionTypeValid])
	updatedCondition := d.newCondition(true, updateTime, datadoghq.DatadogMetricConditionTypeUpdated, existingConditions[datadoghq.DatadogMetricConditionTypeUpdated])
	errorCondition := d.newCondition(d.Error != nil, updateTime, datadoghq.DatadogMetricConditionTypeError, existingConditions[datadoghq.DatadogMetricConditionTypeError])
	if d.Error != nil {
		errorCondition.Reason = DatadogMetricErrorConditionReason
		errorCondition.Message = d.Error.Error()
	}

	newStatus := datadoghq.DatadogMetricStatus{
		Value:      d.Value,
		Conditions: []datadoghq.DatadogMetricCondition{activeCondition, validCondition, updatedCondition, errorCondition},
	}

	return &newStatus
}

// ToExternalMetricFormat returns the current DatadogMetric in the format used by Kubernetes
func (d *DatadogMetricInternal) ToExternalMetricFormat(externalMetricName string) (*external_metrics.ExternalMetricValue, error) {
	if !d.Valid {
		return nil, fmt.Errorf("DatadogMetric is invalid, err: %v", d.Error)
	}

	quantity, err := resource.ParseQuantity(fmt.Sprintf("%v", d.Value))
	if err != nil {
		return nil, err
	}

	return &external_metrics.ExternalMetricValue{
		MetricName:   externalMetricName,
		MetricLabels: nil,
		Value:        quantity,
		Timestamp:    metav1.NewTime(d.UpdateTime),
	}, nil
}

func (d *DatadogMetricInternal) newCondition(status bool, updateTime metav1.Time, conditionType datadoghq.DatadogMetricConditionType, prevCondition *datadoghq.DatadogMetricCondition) datadoghq.DatadogMetricCondition {
	condition := datadoghq.DatadogMetricCondition{
		Type:           conditionType,
		Status:         corev1.ConditionFalse,
		LastUpdateTime: updateTime,
	}

	if status {
		condition.Status = corev1.ConditionTrue
	}

	if prevCondition == nil || (prevCondition != nil && prevCondition.Status != condition.Status) {
		condition.LastTransitionTime = updateTime
	} else {
		condition.LastTransitionTime = prevCondition.LastTransitionTime
	}

	return condition
}
