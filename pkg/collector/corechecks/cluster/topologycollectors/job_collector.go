// +build kubeapiserver

package topologycollectors

import (
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	v1 "k8s.io/api/batch/v1"
)

// JobCollector implements the ClusterTopologyCollector interface.
type JobCollector struct {
	ComponentChan chan<- *topology.Component
	RelationChan  chan<- *topology.Relation
	ClusterTopologyCollector
}

// NewJobCollector
func NewJobCollector(componentChannel chan<- *topology.Component, relationChannel chan<- *topology.Relation, clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {
	return &JobCollector{
		ComponentChan:            componentChannel,
		RelationChan:             relationChannel,
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// GetName returns the name of the Collector
func (_ *JobCollector) GetName() string {
	return "Job Collector"
}

// Collects and Published the Job Components
func (jc *JobCollector) CollectorFunction() error {
	jobs, err := jc.GetAPIClient().GetJobs()
	if err != nil {
		return err
	}

	for _, job := range jobs {
		component := jc.jobToStackStateComponent(job)
		jc.ComponentChan <- component

		// Create relation to the cron job
		for _, ref := range job.OwnerReferences {
			switch kind := ref.Kind; kind {
			case CronJob:
				cronJobExternalID := jc.buildCronJobExternalID(ref.Name)
				jc.RelationChan <- jc.cronJobToJobStackStateRelation(cronJobExternalID, component.ExternalID)
			}
		}
	}

	return nil
}

// Creates a StackState Job component from a Kubernetes / OpenShift Cluster
func (jc *JobCollector) jobToStackStateComponent(job v1.Job) *topology.Component {
	log.Tracef("Mapping Job to StackState component: %s", job.String())

	tags := emptyIfNil(job.Labels)
	tags = jc.addClusterNameTag(tags)

	jobExternalID := jc.buildJobExternalID(job.Name)
	component := &topology.Component{
		ExternalID: jobExternalID,
		Type:       topology.Type{Name: "job"},
		Data: map[string]interface{}{
			"name":              job.Name,
			"creationTimestamp": job.CreationTimestamp,
			"tags":              tags,
			"namespace":         job.Namespace,
			"uid":               job.UID,
			"backoffLimit":      job.Spec.BackoffLimit,
			"parallelism":       job.Spec.Parallelism,
		},
	}

	component.Data.PutNonEmpty("generateName", job.GenerateName)
	component.Data.PutNonEmpty("kind", job.Kind)

	log.Tracef("Created StackState Job component %s: %v", jobExternalID, component.JSONString())

	return component
}

// Creates a StackState relation from a Kubernetes / OpenShift Job to CronJob relation
func (jc *JobCollector) cronJobToJobStackStateRelation(cronJobExternalID, jobExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes cron job to job relation: %s -> %s", cronJobExternalID, jobExternalID)

	relation := jc.CreateRelation(cronJobExternalID, jobExternalID, "creates")

	log.Tracef("Created StackState cron job -> job relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}
