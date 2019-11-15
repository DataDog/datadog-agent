// Copyright 2013 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mapper

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/statsd_exporter/pkg/mapper/fsm"
	yaml "gopkg.in/yaml.v2"
	"time"
)

var (
	statsdMetricRE    = `[a-zA-Z_](-?[a-zA-Z0-9_])+`
	templateReplaceRE = `(\$\{?\d+\}?)`

	metricLineRE = regexp.MustCompile(`^(\*\.|` + statsdMetricRE + `\.)+(\*|` + statsdMetricRE + `)$`)
	metricNameRE = regexp.MustCompile(`^([a-zA-Z_]|` + templateReplaceRE + `)([a-zA-Z0-9_]|` + templateReplaceRE + `)*$`)
	labelNameRE  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]+$`)
)

type mapperConfigDefaults struct {
	TimerType           TimerType         `yaml:"timer_type"`
	Buckets             []float64         `yaml:"buckets"`
	Quantiles           []metricObjective `yaml:"quantiles"`
	MatchType           MatchType         `yaml:"match_type"`
	GlobDisableOrdering bool              `yaml:"glob_disable_ordering"`
	Ttl                 time.Duration     `yaml:"ttl"`
}

type MetricMapper struct {
	Defaults mapperConfigDefaults `yaml:"defaults"`
	Mappings []MetricMapping      `yaml:"mappings"`
	FSM      *fsm.FSM
	doFSM    bool
	doRegex  bool
	cache    MetricMapperCache
	mutex    sync.RWMutex

	MappingsCount prometheus.Gauge
}

type MetricMapping struct {
	Match           string `yaml:"match"`
	Name            string `yaml:"name"`
	nameFormatter   *fsm.TemplateFormatter
	regex           *regexp.Regexp
	Labels          prometheus.Labels `yaml:"labels"`
	labelKeys       []string
	labelFormatters []*fsm.TemplateFormatter
	TimerType       TimerType         `yaml:"timer_type"`
	Buckets         []float64         `yaml:"buckets"`
	Quantiles       []metricObjective `yaml:"quantiles"`
	MatchType       MatchType         `yaml:"match_type"`
	HelpText        string            `yaml:"help"`
	Action          ActionType        `yaml:"action"`
	MatchMetricType MetricType        `yaml:"match_metric_type"`
	Ttl             time.Duration     `yaml:"ttl"`
}

type metricObjective struct {
	Quantile float64 `yaml:"quantile"`
	Error    float64 `yaml:"error"`
}

var defaultQuantiles = []metricObjective{
	{Quantile: 0.5, Error: 0.05},
	{Quantile: 0.9, Error: 0.01},
	{Quantile: 0.99, Error: 0.001},
}

func (m *MetricMapper) InitFromYAMLString(fileContents string, cacheSize int) error {
	var n MetricMapper

	if err := yaml.Unmarshal([]byte(fileContents), &n); err != nil {
		return err
	}

	if n.Defaults.Buckets == nil || len(n.Defaults.Buckets) == 0 {
		n.Defaults.Buckets = prometheus.DefBuckets
	}

	if n.Defaults.Quantiles == nil || len(n.Defaults.Quantiles) == 0 {
		n.Defaults.Quantiles = defaultQuantiles
	}

	if n.Defaults.MatchType == MatchTypeDefault {
		n.Defaults.MatchType = MatchTypeGlob
	}

	remainingMappingsCount := len(n.Mappings)

	n.FSM = fsm.NewFSM([]string{string(MetricTypeCounter), string(MetricTypeGauge), string(MetricTypeTimer)},
		remainingMappingsCount, n.Defaults.GlobDisableOrdering)

	for i := range n.Mappings {
		remainingMappingsCount--

		currentMapping := &n.Mappings[i]

		// check that label is correct
		for k := range currentMapping.Labels {
			if !labelNameRE.MatchString(k) {
				return fmt.Errorf("invalid label key: %s", k)
			}
		}

		if currentMapping.Name == "" {
			return fmt.Errorf("line %d: metric mapping didn't set a metric name", i)
		}

		if !metricNameRE.MatchString(currentMapping.Name) {
			return fmt.Errorf("metric name '%s' doesn't match regex '%s'", currentMapping.Name, metricNameRE)
		}

		if currentMapping.MatchType == "" {
			currentMapping.MatchType = n.Defaults.MatchType
		}

		if currentMapping.Action == "" {
			currentMapping.Action = ActionTypeMap
		}

		if currentMapping.MatchType == MatchTypeGlob {
			n.doFSM = true
			if !metricLineRE.MatchString(currentMapping.Match) {
				return fmt.Errorf("invalid match: %s", currentMapping.Match)
			}

			captureCount := n.FSM.AddState(currentMapping.Match, string(currentMapping.MatchMetricType),
				remainingMappingsCount, currentMapping)

			currentMapping.nameFormatter = fsm.NewTemplateFormatter(currentMapping.Name, captureCount)

			labelKeys := make([]string, len(currentMapping.Labels))
			labelFormatters := make([]*fsm.TemplateFormatter, len(currentMapping.Labels))
			labelIndex := 0
			for label, valueExpr := range currentMapping.Labels {
				labelKeys[labelIndex] = label
				labelFormatters[labelIndex] = fsm.NewTemplateFormatter(valueExpr, captureCount)
				labelIndex++
			}
			currentMapping.labelFormatters = labelFormatters
			currentMapping.labelKeys = labelKeys

		} else {
			if regex, err := regexp.Compile(currentMapping.Match); err != nil {
				return fmt.Errorf("invalid regex %s in mapping: %v", currentMapping.Match, err)
			} else {
				currentMapping.regex = regex
			}
			n.doRegex = true
		}

		if currentMapping.TimerType == "" {
			currentMapping.TimerType = n.Defaults.TimerType
		}

		if currentMapping.Buckets == nil || len(currentMapping.Buckets) == 0 {
			currentMapping.Buckets = n.Defaults.Buckets
		}

		if currentMapping.Quantiles == nil || len(currentMapping.Quantiles) == 0 {
			currentMapping.Quantiles = n.Defaults.Quantiles
		}

		if currentMapping.Ttl == 0 && n.Defaults.Ttl > 0 {
			currentMapping.Ttl = n.Defaults.Ttl
		}

	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.Defaults = n.Defaults
	m.Mappings = n.Mappings
	m.InitCache(cacheSize)

	if n.doFSM {
		var mappings []string
		for _, mapping := range n.Mappings {
			if mapping.MatchType == MatchTypeGlob {
				mappings = append(mappings, mapping.Match)
			}
		}
		n.FSM.BacktrackingNeeded = fsm.TestIfNeedBacktracking(mappings, n.FSM.OrderingDisabled)

		m.FSM = n.FSM
		m.doRegex = n.doRegex
	}
	m.doFSM = n.doFSM

	if m.MappingsCount != nil {
		m.MappingsCount.Set(float64(len(n.Mappings)))
	}
	return nil
}

func (m *MetricMapper) InitFromFile(fileName string, cacheSize int) error {
	mappingStr, err := ioutil.ReadFile(fileName)
	if err != nil {
		return err
	}

	return m.InitFromYAMLString(string(mappingStr), cacheSize)
}

func (m *MetricMapper) InitCache(cacheSize int) {
	if cacheSize == 0 {
		m.cache = NewMetricMapperNoopCache()
	} else {
		cache, err := NewMetricMapperCache(cacheSize)
		if err != nil {
			log.Fatalf("Unable to setup metric cache. Caused by: %s", err)
		}
		m.cache = cache
	}
}

func (m *MetricMapper) GetMapping(statsdMetric string, statsdMetricType MetricType) (*MetricMapping, prometheus.Labels, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	result, cached := m.cache.Get(statsdMetric, statsdMetricType)
	if cached {
		return result.Mapping, result.Labels, result.Matched
	}
	// glob matching
	if m.doFSM {
		finalState, captures := m.FSM.GetMapping(statsdMetric, string(statsdMetricType))
		if finalState != nil && finalState.Result != nil {
			result := finalState.Result.(*MetricMapping)
			result.Name = result.nameFormatter.Format(captures)

			labels := prometheus.Labels{}
			for index, formatter := range result.labelFormatters {
				labels[result.labelKeys[index]] = formatter.Format(captures)
			}

			m.cache.AddMatch(statsdMetric, statsdMetricType, result, labels)

			return result, labels, true
		} else if !m.doRegex {
			// if there's no regex match type, return immediately
			m.cache.AddMiss(statsdMetric, statsdMetricType)
			return nil, nil, false
		}
	}

	// regex matching
	for _, mapping := range m.Mappings {
		// if a rule don't have regex matching type, the regex field is unset
		if mapping.regex == nil {
			continue
		}
		matches := mapping.regex.FindStringSubmatchIndex(statsdMetric)
		if len(matches) == 0 {
			continue
		}

		mapping.Name = string(mapping.regex.ExpandString(
			[]byte{},
			mapping.Name,
			statsdMetric,
			matches,
		))

		if mt := mapping.MatchMetricType; mt != "" && mt != statsdMetricType {
			continue
		}

		labels := prometheus.Labels{}
		for label, valueExpr := range mapping.Labels {
			value := mapping.regex.ExpandString([]byte{}, valueExpr, statsdMetric, matches)
			labels[label] = string(value)
		}

		m.cache.AddMatch(statsdMetric, statsdMetricType, &mapping, labels)

		return &mapping, labels, true
	}

	m.cache.AddMiss(statsdMetric, statsdMetricType)
	return nil, nil, false
}
