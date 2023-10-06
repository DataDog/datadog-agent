// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"context"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	basemetrics "k8s.io/component-base/metrics"
	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var descJobLabelsDefaultLabels = []string{"namespace", "job_name"}

// NewExtendedJobFactory returns a new Job metric family generator factory.
func NewExtendedJobFactory(client *apiserver.APIClient) customresource.RegistryFactory {
	return &extendedJobFactory{
		client: client.Cl,
	}
}

type extendedJobFactory struct {
	client interface{}
}

// Name is the name of the factory
func (f *extendedJobFactory) Name() string {
	return "jobs_extended"
}

// CreateClient is not implemented
func (f *extendedJobFactory) CreateClient(cfg *rest.Config) (interface{}, error) {
	return f.client, nil
}

// MetricFamilyGenerators returns the extended job metric family generators
func (f *extendedJobFactory) MetricFamilyGenerators(allowAnnotationsList, allowLabelsList []string) []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			"kube_job_duration",
			"Duration represents the time elapsed between the StartTime and CompletionTime of a Job, or the current time if the job is still running",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapJobFunc(func(j *batchv1.Job) *metric.Family {
				ms := []*metric.Metric{}

				if j.Status.StartTime != nil {
					start := j.Status.StartTime.Unix()
					end := time.Now().Unix()

					if j.Status.CompletionTime != nil {
						end = j.Status.CompletionTime.Unix()
					}

					ms = append(ms, &metric.Metric{
						Value: float64(end - start),
					})
				}

				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
	}
}

func wrapJobFunc(f func(*batchv1.Job) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		job, ok := obj.(*batchv1.Job)
		if !ok {
			log.Warnf("cannot cast object %T into *batchv1.Job, skipping", obj)
			return nil
		}

		metricFamily := f(job)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys, m.LabelValues = mergeKeyValues(descJobLabelsDefaultLabels, []string{job.Namespace, job.Name}, m.LabelKeys, m.LabelValues)
		}

		return metricFamily
	}
}

// ExpectedType returns the type expected by the factory
func (f *extendedJobFactory) ExpectedType() interface{} {
	return &batchv1.Job{}
}

// ListWatch returns a ListerWatcher for batchv1.Job
func (f *extendedJobFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	client := customResourceClient.(kubernetes.Interface)
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = fieldSelector
			return client.BatchV1().Jobs(ns).List(context.TODO(), opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = fieldSelector
			return client.BatchV1().Jobs(ns).Watch(context.TODO(), opts)
		},
	}
}
