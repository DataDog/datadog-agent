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
	config map[string]*JoinsConfig
	forest map[string]*node
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
	return &labelJoiner{
		config: config,
		forest: make(map[string]*node),
	}
}

func newNode() *node {
	return &node{
		labelValues: make(map[string]*node),
		//labelsToAdd: make([]label),
	}
}

func (lj *labelJoiner) insertMetric(metric ksmstore.DDMetric, config *JoinsConfig, tree *node) {
	current := tree
	for _, labelToMatch := range config.LabelsToMatch {
		labelValue, found := metric.Labels[labelToMatch]
		if !found {
			return
		}

		child, found := current.labelValues[labelValue]
		if !found {
			child = newNode()
			current.labelValues[labelValue] = child
		}

		current = child
	}

	if config.GetAllLabels {
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
		for _, labelToGet := range config.LabelsToGet {
			labelValue, found := metric.Labels[labelToGet]
			if found {
				current.labelsToAdd = append(current.labelsToAdd, label{labelToGet, labelValue})
			}
		}
	}
}

func (lj *labelJoiner) insertFamily(metricFamily ksmstore.DDMetricsFam) {
	config, found := lj.config[metricFamily.Name]
	if !found {
		log.Error("BUG in label joins")
		return
	}

	tree, found := lj.forest[metricFamily.Name]
	if !found {
		tree = newNode()
		lj.forest[metricFamily.Name] = tree
	}

	for _, metric := range metricFamily.ListMetrics {
		lj.insertMetric(metric, config, tree)
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
	for metricName, tree := range lj.forest {
		config, found := lj.config[metricName]
		if !found {
			log.Error("BUG in label joins")
			return
		}

		lj.getLabelsToAddOne(inputLabels, config, tree, &labelsToAdd)
	}

	return
}
