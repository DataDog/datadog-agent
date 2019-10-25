// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package kubeapi

import (
	"github.com/StackVista/stackstate-agent/pkg/autodiscovery/integration"
	"github.com/StackVista/stackstate-agent/pkg/batcher"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	core "github.com/StackVista/stackstate-agent/pkg/collector/corechecks"
	collectors "github.com/StackVista/stackstate-agent/pkg/collector/corechecks/cluster/topology_collectors"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"sync"
)

const (
	kubernetesAPITopologyCheckName = "kubernetes_api_topology"
)

// TopologyCheck grabs events from the API server.
type TopologyCheck struct {
	CommonCheck
	instance *TopologyConfig
}

// Configure parses the check configuration and init the check.
func (t *TopologyCheck) Configure(config, initConfig integration.Data) error {
	err := t.ConfigureKubeApiCheck(config)
	if err != nil {
		return err
	}

	err = t.instance.parse(config)
	if err != nil {
		_ = log.Error("could not parse the config for the API topology check")
		return err
	}

	log.Debugf("Running config %s", config)
	return nil
}

// Run executes the check.
func (t *TopologyCheck) Run() error {
	// initialize kube api check
	err := t.InitKubeApiCheck()
	if err == apiserver.ErrNotLeader {
		log.Debug("Agent is not leader, will not run the check")
		return nil
	} else if err != nil {
		return err
	}

	// Running the event collection.
	if !t.instance.CollectTopology {
		return nil
	}

	// set the check "instance id" for snapshots
	t.instance.CheckID = kubernetesAPITopologyCheckName

	var instanceClusterType ClusterType
	switch openshiftPresence := t.ac.DetectOpenShiftAPILevel(); openshiftPresence {
	case apiserver.OpenShiftAPIGroup, apiserver.OpenShiftOAPI:
		instanceClusterType = OpenShift
	case apiserver.NotOpenShift:
		instanceClusterType = Kubernetes
	}
	t.instance.Instance = topology.Instance{Type: string(instanceClusterType), URL: t.instance.ClusterName}

	// start the topology snapshot with the batch-er
	batcher.GetBatcher().SubmitStartSnapshot(t.instance.CheckID, t.instance.Instance)

	// set up a WaitGroup to wait for the concurrent topology gathering of the functions below
	var wg sync.WaitGroup

	var clusterCollectors []collectors.ClusterTopologyCollector

	// Make a channel for each of the relations to avoid passing data down into all the functions
	containerCorrelationChannel := make(chan *collectors.ContainerCorrelation)

	// make a channel that is responsible for publishing components and relations
	componentChannel := make(chan *topology.Component)
	relationChannel := make(chan *topology.Relation)
	errChannel := make(chan error)

	/*
		cluster -> map cluster -> component

		node -> map node -> component
					     -> cluster relation
			   component <- container correlator
				relation <-

		pod -> map pod 	  		 -> component
								 -> node relation
			container correlator <- map func container -> component
													   -> relation

		service -> map service -> component
							   -> endpoints as identifiers
							   -> pod relation

		component -> publish component
		relation -> publish relation
	*/

	//
	commonClusterCollector := collectors.NewClusterTopologyCollector(t.instance.Instance, t.ac)

	clusterCollectors = append(clusterCollectors,
		// Register Cluster Component Collector
		collectors.NewClusterCollector(
			componentChannel,
			commonClusterCollector,
		),
		// Register ConfigMap Component Collector
		collectors.NewConfigMapCollector(
			componentChannel,
			relationChannel,
			commonClusterCollector,
		),
		// Register CronJob Component Collector
		collectors.NewCronJobCollector(
			componentChannel,
			relationChannel,
			commonClusterCollector,
		),
		// Register DaemonSet Component Collector
		collectors.NewDaemonSetCollector(
			componentChannel,
			commonClusterCollector,
		),
		// Register Deployment Component Collector
		collectors.NewDeploymentCollector(
			componentChannel,
			commonClusterCollector,
		),
		// Register Ingress Component Collector
		collectors.NewIngressCollector(
			componentChannel,
			relationChannel,
			commonClusterCollector,
		),
		// Register Job Component Collector
		collectors.NewJobCollector(
			componentChannel,
			relationChannel,
			commonClusterCollector,
		),
		// Register Node Component Collector
		collectors.NewNodeCollector(
			componentChannel,
			relationChannel,
			containerCorrelationChannel,
			commonClusterCollector,
		),
		// Register Persistent Volume Component Collector
		collectors.NewPersistentVolumeCollector(
			componentChannel,
			relationChannel,
			commonClusterCollector,
		),
		// Register Pod Component Collector
		collectors.NewPodCollector(
			componentChannel,
			relationChannel,
			containerCorrelationChannel,
			commonClusterCollector,
		),
		// Register ReplicaSet Component Collector
		collectors.NewReplicaSetCollector(
			componentChannel,
			commonClusterCollector,
		),
		// Register Service Component Collector
		collectors.NewServiceCollector(
			componentChannel,
			relationChannel,
			commonClusterCollector,
		),
		// Register StatefulSet Component Collector
		collectors.NewStatefulSetCollector(
			componentChannel,
			commonClusterCollector,
		),
		// Register Persistent Volume Component Collector
		collectors.NewVolumeCollector(
			componentChannel,
			relationChannel,
			commonClusterCollector,
		),
	)

	wg.Add(len(clusterCollectors))

	for _, collector := range clusterCollectors {
		go func(col collectors.ClusterTopologyCollector) {
			defer wg.Done()
			log.Debugf("Starting cluster topology collector: %s", col.GetName())
			err := col.CollectorFunction()
			if err != nil {
				errChannel <- err
			}
		}(collector)
	}

	go func() {
		// publish all incoming components
		for component := range componentChannel {
			log.Debugf("Publishing StackState cluster component for %s: %v", component.ExternalID, component.JSONString())
			batcher.GetBatcher().SubmitComponent(t.instance.CheckID, t.instance.Instance, *component)
		}

		// publish all incoming relations
		for relation := range relationChannel {
			log.Debugf("Publishing StackState node -> cluster relation %s->%s", relation.SourceID, relation.TargetID)
			batcher.GetBatcher().SubmitRelation(t.instance.CheckID, t.instance.Instance, *relation)
		}

		// publish all incoming errors
		for err := range errChannel {
			_ = log.Error(err)
		}
	}()

	wg.Wait()

	defer close(containerCorrelationChannel)
	defer close(componentChannel)
	defer close(relationChannel)
	defer close(errChannel)

	// get all the containers
	batcher.GetBatcher().SubmitStopSnapshot(t.instance.CheckID, t.instance.Instance)
	batcher.GetBatcher().SubmitComplete(t.instance.CheckID)

	return nil
}

// KubernetesASFactory is exported for integration testing.
func KubernetesApiTopologyFactory() check.Check {
	return &TopologyCheck{
		CommonCheck: CommonCheck{
			CheckBase: core.NewCheckBase(kubernetesAPITopologyCheckName),
		},
		instance: &TopologyConfig{},
	}
}

func init() {
	core.RegisterCheck(kubernetesAPITopologyCheckName, KubernetesApiTopologyFactory)
}
