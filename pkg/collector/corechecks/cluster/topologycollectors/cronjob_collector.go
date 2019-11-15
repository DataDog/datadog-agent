// +build kubeapiserver

package topologycollectors

import (
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"k8s.io/api/batch/v1beta1"
)

// CronJobCollector implements the ClusterTopologyCollector interface.
type CronJobCollector struct {
	ComponentChan chan<- *topology.Component
	ClusterTopologyCollector
}

// NewCronJobCollector
func NewCronJobCollector(componentChannel chan<- *topology.Component, clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {
	return &CronJobCollector{
		ComponentChan:            componentChannel,
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// GetName returns the name of the Collector
func (_ *CronJobCollector) GetName() string {
	return "CronJob Collector"
}

// Collects and Published the Cron Job Components
func (cjc *CronJobCollector) CollectorFunction() error {
	cronJobs, err := cjc.GetAPIClient().GetCronJobs()
	if err != nil {
		return err
	}

	for _, cj := range cronJobs {
		component := cjc.cronJobToStackStateComponent(cj)
		cjc.ComponentChan <- component
	}

	return nil
}

// Creates a StackState CronJob component from a Kubernetes / OpenShift Cluster
func (cjc *CronJobCollector) cronJobToStackStateComponent(cronJob v1beta1.CronJob) *topology.Component {
	log.Tracef("Mapping CronJob to StackState component: %s", cronJob.String())

	tags := emptyIfNil(cronJob.Labels)
	tags = cjc.addClusterNameTag(tags)

	cronJobExternalID := cjc.buildCronJobExternalID(cronJob.Name)
	component := &topology.Component{
		ExternalID: cronJobExternalID,
		Type:       topology.Type{Name: "cronjob"},
		Data: map[string]interface{}{
			"name":              cronJob.Name,
			"creationTimestamp": cronJob.CreationTimestamp,
			"tags":              tags,
			"namespace":         cronJob.Namespace,
			"uid":               cronJob.UID,
			"concurrencyPolicy": cronJob.Spec.ConcurrencyPolicy,
			"schedule":          cronJob.Spec.Schedule,
		},
	}

	component.Data.PutNonEmpty("generateName", cronJob.GenerateName)
	component.Data.PutNonEmpty("kind", cronJob.Kind)

	log.Tracef("Created StackState CronJob component %s: %v", cronJobExternalID, component.JSONString())

	return component
}
