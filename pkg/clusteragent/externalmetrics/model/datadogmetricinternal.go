// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

// exported for testing purposes
const (
	DatadogMetricErrorConditionReason string = "Unable to fetch data from Datadog"
	alwaysActiveAnnotation            string = "external-metrics.datadoghq.com/always-active"
)

// DatadogMetricInternal is a flatten, easier to use, representation of `DatadogMetric` CRD
type DatadogMetricInternal struct {
	ID                   string
	query                string
	resolvedQuery        *string
	Valid                bool
	Active               bool
	AlwaysActive         bool
	Deleted              bool
	Autogen              bool
	ExternalMetricName   string
	Value                float64
	AutoscalerReferences string
	UpdateTime           time.Time
	DataTime             time.Time
	Error                error
	MaxAge               time.Duration
	TimeWindow           time.Duration
}

// NewDatadogMetricInternal returns a `DatadogMetricInternal` object from a `DatadogMetric` CRD Object
// `id` is expected to be unique and should correspond to `namespace/name`
func NewDatadogMetricInternal(id string, datadogMetric datadoghq.DatadogMetric) DatadogMetricInternal {
	internal := DatadogMetricInternal{
		ID:                   id,
		query:                datadogMetric.Spec.Query,
		Valid:                false,
		Active:               false,
		AlwaysActive:         hasForceActiveAnnotation(datadogMetric),
		Deleted:              false,
		Autogen:              false,
		AutoscalerReferences: datadogMetric.Status.AutoscalerReferences,
		MaxAge:               datadogMetric.Spec.MaxAge.Duration,
		TimeWindow:           datadogMetric.Spec.TimeWindow.Duration,
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
			internal.UpdateTime = condition.LastUpdateTime.UTC()
		case condition.Type == datadoghq.DatadogMetricConditionTypeUpdated && condition.Status == corev1.ConditionTrue:
			internal.DataTime = condition.LastUpdateTime.UTC()
		case condition.Type == datadoghq.DatadogMetricConditionTypeError && condition.Status == corev1.ConditionTrue:
			internal.Error = errors.New(condition.Message)
		}
	}

	internal.resolveQuery(internal.query)

	// If UpdateTime is not set, it means it's a newly created DatadogMetric
	// We'll need a proper update time to generate status, so setting to current time
	if internal.UpdateTime.IsZero() {
		internal.UpdateTime = time.Now().UTC()
	}

	// Handling value last as we may invalidate DatadogMetric if we get a parsing error
	value, err := parseDatadogMetricValue(datadogMetric.Status.Value)
	if err != nil {
		log.Errorf("Unable to parse DatadogMetric value from string: '%s', invalidating: %s", datadogMetric.Status.Value, id)
		internal.Valid = false
		internal.UpdateTime = time.Now().UTC()
		value = 0
	}
	internal.Value = value

	return internal
}

func hasForceActiveAnnotation(metric datadoghq.DatadogMetric) bool {
	if value, found := metric.Annotations[alwaysActiveAnnotation]; found {
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			log.Debugf("Unable to parse value from %s annotation: '%s'", alwaysActiveAnnotation, value)
			return false
		}
		return enabled
	}
	return false
}

// NewDatadogMetricInternalFromExternalMetric returns a `DatadogMetricInternal` object
// that is auto-generated from a standard ExternalMetric query (non-DatadogMetric reference)
func NewDatadogMetricInternalFromExternalMetric(id, query, metricName, autoscalerReference string) DatadogMetricInternal {
	return DatadogMetricInternal{
		ID:                   id,
		query:                query,
		Valid:                false,
		Active:               true,
		AlwaysActive:         false,
		Deleted:              false,
		Autogen:              true,
		ExternalMetricName:   metricName,
		AutoscalerReferences: autoscalerReference,
		UpdateTime:           time.Now().UTC(),
	}
}

// query returns the query that should be used to fetch metrics
func (d *DatadogMetricInternal) Query() string {
	if d.resolvedQuery != nil {
		return *d.resolvedQuery
	}
	return d.query
}

// RawQuery returns the query that should be used to create DDM objects
func (d *DatadogMetricInternal) RawQuery() string {
	return d.query
}

// UpdateFrom updates the `DatadogMetricInternal` from `DatadogMetric`
func (d *DatadogMetricInternal) UpdateFrom(current datadoghq.DatadogMetric) {
	currentSpec := current.Spec

	if d.shouldResolveQuery(currentSpec) {
		d.resolveQuery(currentSpec.Query)
	}
	d.query = currentSpec.Query
	d.MaxAge = currentSpec.MaxAge.Duration
	d.TimeWindow = currentSpec.TimeWindow.Duration
	d.AlwaysActive = hasForceActiveAnnotation(current)
}

// GetTimeWindow gets the time window for the metric, if unset defaults to max age.
func (d *DatadogMetricInternal) GetTimeWindow() time.Duration {
	timeWindow := d.TimeWindow
	if timeWindow == 0 {
		timeWindow = d.MaxAge
	}
	return timeWindow
}

// shouldResolveQuery returns whether we should try to resolve a new query
func (d *DatadogMetricInternal) shouldResolveQuery(spec datadoghq.DatadogMetricSpec) bool {
	return d.resolvedQuery == nil || d.query != spec.Query
}

// IsNewerThan returns true if the current `DatadogMetricInternal` has been updated more recently than `DatadogMetric` Status
func (d *DatadogMetricInternal) IsNewerThan(currentStatus datadoghq.DatadogMetricStatus) bool {
	for _, condition := range currentStatus.Conditions {
		// Any condition can be used, except DatadogMetricConditionTypeUpdated as this one is updated with `DataTime` instead
		if condition.Type == datadoghq.DatadogMetricConditionTypeActive {
			if condition.LastUpdateTime.UTC().Unix() >= d.UpdateTime.UTC().Unix() {
				return false
			}
			break
		}
	}

	return true
}

// HasBeenUpdatedFor returns true if the current `DatadogMetricInternal` has been update between Now() and Now() - duration
func (d *DatadogMetricInternal) HasBeenUpdatedFor(duration time.Duration) bool {
	return d.UpdateTime.After(time.Now().UTC().Add(-duration))
}

// BuildStatus generates a new status for `DatadogMetric` based on current status and information from `DatadogMetricInternal`
// The updated condition refers to the Value update time (datapoint timestamp from Datadog API).
func (d *DatadogMetricInternal) BuildStatus(currentStatus *datadoghq.DatadogMetricStatus) *datadoghq.DatadogMetricStatus {
	updateTime := metav1.NewTime(d.UpdateTime)
	dataTime := metav1.NewTime(d.DataTime)

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
	updatedCondition := d.newCondition(true, dataTime, datadoghq.DatadogMetricConditionTypeUpdated, existingConditions[datadoghq.DatadogMetricConditionTypeUpdated])
	errorCondition := d.newCondition(d.Error != nil, updateTime, datadoghq.DatadogMetricConditionTypeError, existingConditions[datadoghq.DatadogMetricConditionTypeError])
	if d.Error != nil {
		errorCondition.Reason = DatadogMetricErrorConditionReason
		errorCondition.Message = d.Error.Error()
	}

	newStatus := datadoghq.DatadogMetricStatus{
		Value:                formatDatadogMetricValue(d.Value),
		Conditions:           []datadoghq.DatadogMetricCondition{activeCondition, validCondition, updatedCondition, errorCondition},
		AutoscalerReferences: d.AutoscalerReferences,
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
		Timestamp:    metav1.NewTime(d.DataTime),
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

// resolveQuery tries to resolve the query and set the DatadogMetricInternal fields accordingly
func (d *DatadogMetricInternal) resolveQuery(query string) {
	resolvedQuery, err := resolveQuery(query)
	if err != nil {
		log.Errorf("Unable to resolve DatadogMetric query %q: %v", d.query, err)
		d.Valid = false
		d.Error = fmt.Errorf("Cannot resolve query: %v", err)
		d.UpdateTime = time.Now().UTC()
		d.resolvedQuery = nil
		return
	}
	if resolvedQuery != "" {
		log.Infof("DatadogMetric query %q was resolved successfully, new query: %q", query, resolvedQuery)
		d.resolvedQuery = &resolvedQuery
		return
	}
	d.resolvedQuery = &d.query
}

// SetQueries is only used for testing in other packages
func (d *DatadogMetricInternal) SetQueries(q string) {
	d.query = q
	d.resolvedQuery = &q
}

// SetQuery is only used for testing in other packages
func (d *DatadogMetricInternal) SetQuery(q string) {
	d.query = q
}
