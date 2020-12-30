// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package cluster

import (
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type labelJoiner struct {
	metricsToJoin map[string]metricToJoin
}

type metricToJoin struct {
	config *JoinsConfig
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

func newLabelJoiner(config map[string]*JoinsConfig) *labelJoiner {
	metricsToJoin := make(map[string]metricToJoin)

	for key, joinsConfig := range config {
		if len(joinsConfig.LabelsToMatch) > 0 {
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

func newInnerNode() *node {
	return &node{
		labelValues: make(map[string]*node),
		labelsToAdd: nil,
	}
}

func newLeafNode() *node {
	return &node{
		labelValues: nil,
		labelsToAdd: nil,
	}
}

func (lj *labelJoiner) insertMetric(metric ksmstore.DDMetric, config *JoinsConfig, tree *node) {
	current := tree
	nbLabelsToMatch := len(config.LabelsToMatch)
	for i, labelToMatch := range config.LabelsToMatch {
		labelValue, found := metric.Labels[labelToMatch]
		if !found {
			return
		}

		child, found := current.labelValues[labelValue]
		if !found {
			if i < nbLabelsToMatch-1 {
				child = newInnerNode()
			} else {
				child = newLeafNode()
			}
			current.labelValues[labelValue] = child
		}

		current = child
	}

	if config.GetAllLabels {
		if current.labelsToAdd == nil {
			current.labelsToAdd = make([]label, 0, len(metric.Labels)-len(config.LabelsToMatch))
		}

		for labelName, labelValue := range metric.Labels {
			isALabelToMatch := false
			for _, labelToMatch := range config.LabelsToMatch {
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
			current.labelsToAdd = make([]label, 0, len(config.LabelsToGet))
		}

		for _, labelToGet := range config.LabelsToGet {
			labelValue, found := metric.Labels[labelToGet]
			if found {
				current.labelsToAdd = append(current.labelsToAdd, label{labelToGet, labelValue})
			}
		}
	}
}

func (lj *labelJoiner) insertFamily(metricFamily ksmstore.DDMetricsFam) {
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

func (lj *labelJoiner) getLabelsToAddOne(inputLabels map[string]string, config *JoinsConfig, tree *node, labelsToAdd *[]label) {
	node := tree
	for _, labelToMatch := range config.LabelsToMatch {
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
