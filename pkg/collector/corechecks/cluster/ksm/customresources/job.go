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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	basemetrics "k8s.io/component-base/metrics"
	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var descJobLabelsDefaultLabels = []string{"namespace", "job_name"}

// NewExtendedJobFactory returns a new Job metric family generator factory.
func NewExtendedJobFactory(client *dynamic.DynamicClient) customresource.RegistryFactory {
	return &extendedJobFactory{
		client: client,
	}
}

type extendedJobFactory struct {
	client *dynamic.DynamicClient
}

// Name is the name of the factory
func (f *extendedJobFactory) Name() string {
	return "jobs_extended"
}

func (f *extendedJobFactory) CreateClient(_ *rest.Config) (interface{}, error) {
	return f.client.Resource(schema.GroupVersionResource{
		Group:    batchv1.GroupName,
		Version:  batchv1.SchemeGroupVersion.Version,
		Resource: "jobs",
	}), nil
}

// MetricFamilyGenerators returns the extended job metric family generators
func (f *extendedJobFactory) MetricFamilyGenerators() []generator.FamilyGenerator {
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
		job := &batchv1.Job{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.(*unstructured.Unstructured).Object, job); err != nil {
			log.Warnf("cannot decode object %q into batchv1.Job, err=%s, skipping", obj.(*unstructured.Unstructured).Object["apiVersion"], err)
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
	u := unstructured.Unstructured{}
	u.SetGroupVersionKind(batchv1.SchemeGroupVersion.WithKind("Job"))
	return &u
}

// ListWatch returns a ListerWatcher for batchv1.Job
func (f *extendedJobFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	client := customResourceClient.(dynamic.NamespaceableResourceInterface).Namespace(ns)
	ctx := context.Background()
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = fieldSelector
			return client.List(ctx, opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = fieldSelector
			return client.Watch(ctx, opts)
		},
	}
}
