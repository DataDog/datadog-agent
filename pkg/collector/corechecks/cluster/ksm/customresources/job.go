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
	log.Infof("ROLLOUT-JOB: NewExtendedJobFactory called")
	return &extendedJobFactory{
		client: client.Cl,
	}
}

type extendedJobFactory struct {
	client kubernetes.Interface
}

// Name is the name of the factory
func (f *extendedJobFactory) Name() string {
	log.Infof("ROLLOUT-JOB: Name() called, returning 'jobs_extended'")
	return "jobs_extended"
}

func (f *extendedJobFactory) CreateClient(_ *rest.Config) (interface{}, error) {
	log.Infof("ROLLOUT-JOB: CreateClient() called")
	return f.client, nil
}

// MetricFamilyGenerators returns the extended job metric family generators
func (f *extendedJobFactory) MetricFamilyGenerators() []generator.FamilyGenerator {
	log.Infof("ROLLOUT-JOB: MetricFamilyGenerators() called")
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			"kube_job_duration",
			"Duration represents the time elapsed between the StartTime and CompletionTime of a Job, or the current time if the job is still running",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapJobFunc(func(j *batchv1.Job) *metric.Family {
				log.Infof("ROLLOUT-JOB: Processing job %s/%s at %s", j.Namespace, j.Name, time.Now().Format("15:04:05"))
				log.Infof("ROLLOUT-JOB: Job status - StartTime: %v, CompletionTime: %v, Active: %d, Succeeded: %d, Failed: %d",
					j.Status.StartTime, j.Status.CompletionTime, j.Status.Active, j.Status.Succeeded, j.Status.Failed)
				ms := []*metric.Metric{}

				if j.Status.StartTime != nil {
					start := j.Status.StartTime.Unix()
					end := time.Now().Unix()

					if j.Status.CompletionTime != nil {
						end = j.Status.CompletionTime.Unix()
					}

					duration := float64(end - start)
					log.Infof("ROLLOUT-JOB: Job %s/%s duration=%.0f seconds (using current time: %t)",
						j.Namespace, j.Name, duration, j.Status.CompletionTime == nil)

					ms = append(ms, &metric.Metric{
						Value: duration,
					})
				} else {
					log.Infof("ROLLOUT-JOB: Job %s/%s has no StartTime", j.Namespace, j.Name)
				}

				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
	}
}

func wrapJobFunc(f func(*batchv1.Job) *metric.Family) func(interface{}) *metric.Family {
	log.Infof("ROLLOUT-JOB: wrapJobFunc called")
	return func(obj interface{}) *metric.Family {
		job := obj.(*batchv1.Job)
		log.Infof("ROLLOUT-JOB: wrapJobFunc processing job %s/%s", job.Namespace, job.Name)

		metricFamily := f(job)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys, m.LabelValues = mergeKeyValues(descJobLabelsDefaultLabels, []string{job.Namespace, job.Name}, m.LabelKeys, m.LabelValues)
		}

		return metricFamily
	}
}

// ExpectedType returns the type expected by the factory
func (f *extendedJobFactory) ExpectedType() interface{} {
	log.Infof("ROLLOUT-JOB: ExpectedType() called")
	return &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: batchv1.SchemeGroupVersion.String(),
		},
	}
}

// ListWatch returns a ListerWatcher for batchv1.Job
func (f *extendedJobFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	log.Infof("ROLLOUT-JOB: ListWatch() called for namespace=%s, fieldSelector=%s", ns, fieldSelector)
	client := customResourceClient.(kubernetes.Interface)
	ctx := context.Background()
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			log.Infof("ROLLOUT-JOB: ListFunc called for namespace=%s", ns)
			opts.FieldSelector = fieldSelector
			result, err := client.BatchV1().Jobs(ns).List(ctx, opts)
			if err != nil {
				log.Warnf("ROLLOUT-JOB: ListFunc error: %v", err)
			} else {
				log.Infof("ROLLOUT-JOB: ListFunc found %d jobs", len(result.Items))
			}
			return result, err
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			log.Infof("ROLLOUT-JOB: WatchFunc called for namespace=%s", ns)
			opts.FieldSelector = fieldSelector
			return client.BatchV1().Jobs(ns).Watch(ctx, opts)
		},
	}
}
