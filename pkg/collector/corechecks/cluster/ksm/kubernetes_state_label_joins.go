// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

/*
   labelJoiner ingests all the metrics used by label joins to build a tree that can then be used to efficiently find which labels should be added to other metrics.

   For ex., for the following JoinsConfig:

     "kube_deployment_labels":
       LabelsToMatch: ["namespace", "deployment"]
       LabelsToGet:   ["chart_name", "chart_version"]

   and the following metrics:

     kube_deployment_labels {"namespace": "ns-a", "deployment": "foo", "chart_name": "foo", "chart_version": "1.0"}
     kube_deployment_labels {"namespace": "ns-a", "deployment": "bar", "chart_name": "bar", "chart_version": "1.1"}
     kube_deployment_labels {"namespace": "ns-b", "deployment": "foo", "chart_name": "foo", "chart_version": "2.0"}

   it will create the following tree:

   kube_deployment_labels
     ├─ ns-a
     │    ├─ foo
     │    │    └─ [ chart_name:foo, chart_version:1.0 ]
     │    └─ bar
     │         └─ [ chart_name:bar, chart_version:1.1 ]
     └─ ns-b
          └─ foo
               └─ [ chart_name:foo, chart_version:2.0 ]

   At the first level of the tree, there are the different values for the "namespace" label (because it’s the first label to match)
   At the second level of the tree, there are all the different values for the "deployment" label (because it’s the first label to match)
   At the third level of the tree, there are the lists of labels to add with the keys and the values.

   When a metric like the following needs to be decorated:

     kube_pod_container_status_running {"namespace": "ns-a", "deployment": "bar", "container": "agent", "pod": "XXX"}

   We first extract the "namespace" value because it’s the first label to match.
   This value is used to lookup the first level node in the tree.
   The "deployment" value is then extracted because it’s the second label to match.
   This value is used to lookup the second level node in the tree.
   That node contains the list of labels to add.
*/

type joinsConfig struct {
	labelsToMatch []string
	labelsToGet   map[string]string
	getAllLabels  bool
}

type labelJoiner struct {
	metricsToJoin map[string]metricToJoin
}

type metricToJoin struct {
	config *joinsConfig
	tree   *node
}

type label struct {
	key   string
	value string
}

type node struct {
	labelValues map[string]*node
	labelsToAdd []label
}

func newLabelJoiner(config map[string]*joinsConfig) *labelJoiner {
	metricsToJoin := make(map[string]metricToJoin)

	for key, joinsConfig := range config {
		if len(joinsConfig.labelsToMatch) > 0 {
			metricsToJoin[key] = metricToJoin{
				config: joinsConfig,
				tree:   newInnerNode(),
			}
		} else {
			metricsToJoin[key] = metricToJoin{
				config: joinsConfig,
				tree:   newLeafNode(),
			}
		}
	}

	return &labelJoiner{
		metricsToJoin: metricsToJoin,
	}
}

// newInnerNode creates a non-leaf node for the tree.
// a non-leaf node has child nodes in the `labelValues` map.
// a non-leaf node doesn’t use `labelsToAdd`.
func newInnerNode() *node {
	return &node{
		labelValues: make(map[string]*node),
		labelsToAdd: nil,
	}
}

// newLeafNode creates a leaf node for the tree.
// a leaf node has no children. So, the `labelValues` map can remain `nil`.
// a leaf node holds a list of labels to add in `labelsToAdd`. But this slice is allocated later, when we know its expected final size.
func newLeafNode() *node {
	return &node{
		labelValues: nil,
		labelsToAdd: nil,
	}
}

func (lj *labelJoiner) insertMetric(metric ksmstore.DDMetric, config *joinsConfig, tree *node) {
	current := tree

	// Parse the tree from the root to the leaf and add missing nodes on the way.
	nbLabelsToMatch := len(config.labelsToMatch)
	for i, labelToMatch := range config.labelsToMatch {
		labelValue, found := metric.Labels[labelToMatch]
		if !found {
			return
		}

		child, found := current.labelValues[labelValue]
		if !found {
			// If the node hasn’t been found in the tree, a node for the current `labelValue` needs to be added.
			// The current depth is checked to know if the node will be a leaf or not.
			if i < nbLabelsToMatch-1 {
				child = newInnerNode()
			} else {
				child = newLeafNode()
			}
			current.labelValues[labelValue] = child
		}

		current = child
	}

	// Fill the `labelsToAdd` on the leaf node.
	if config.getAllLabels {
		if current.labelsToAdd == nil {
			current.labelsToAdd = make([]label, 0, len(metric.Labels)-len(config.labelsToMatch))
		}

		for labelName, labelValue := range metric.Labels {
			isALabelToMatch := false
			for _, labelToMatch := range config.labelsToMatch {
				if labelName == labelToMatch {
					isALabelToMatch = true
					break
				}
			}
			if !isALabelToMatch {
				current.labelsToAdd = append(current.labelsToAdd, label{labelName, labelValue})
			}
		}
	} else {
		if current.labelsToAdd == nil {
			current.labelsToAdd = make([]label, 0, len(config.labelsToGet))
		}

		for labelToGet, ddTagKey := range config.labelsToGet {
			labelValue, found := metric.Labels[labelToGet]
			if found && labelValue != "" {
				current.labelsToAdd = append(current.labelsToAdd, label{ddTagKey, labelValue})
			}
		}
	}
}

func (lj *labelJoiner) insertFamily(metricFamily ksmstore.DDMetricsFam) {
	// The metricsToJoin map has been created in newLabelJoiner and contains one entry per label join config.
	// insertFamily is then called with the metrics to use to do the label join.
	// The metrics passed to insertFamily are retrieved by (*KSMCheck)Run() and are filtered by (*KSMCheck)familyFilter
	// And (*KSMCheck)familyFilter keeps only the metrics that are in the label join config.
	// That’s why we cannot have a miss here.
	metricToJoin, found := lj.metricsToJoin[metricFamily.Name]
	if !found {
		log.Error("BUG in label joins")
		return
	}

	for _, metric := range metricFamily.ListMetrics {
		lj.insertMetric(metric, metricToJoin.config, metricToJoin.tree)
	}
}

func (lj *labelJoiner) insertFamilies(metrics map[string][]ksmstore.DDMetricsFam) {
	for _, metricsList := range metrics {
		for _, metricFamily := range metricsList {
			lj.insertFamily(metricFamily)
		}
	}
}

func (lj *labelJoiner) getLabelsToAddOne(inputLabels map[string]string, config *joinsConfig, tree *node, labelsToAdd *[]label) {
	node := tree
	for _, labelToMatch := range config.labelsToMatch {
		labelValue, found := inputLabels[labelToMatch]
		if !found {
			return
		}

		node, found = node.labelValues[labelValue]
		if !found {
			return
		}
	}

	*labelsToAdd = append(*labelsToAdd, node.labelsToAdd...)
}

func (lj *labelJoiner) getLabelsToAdd(inputLabels map[string]string) (labelsToAdd []label) {
	for _, metricToJoin := range lj.metricsToJoin {
		lj.getLabelsToAddOne(inputLabels, metricToJoin.config, metricToJoin.tree, &labelsToAdd)
	}

	return
}
