// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler_test

import (
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	mockStatsd "github.com/DataDog/datadog-go/v5/statsd/mocks"
	"github.com/golang/mock/gomock"
)

func Test_Metrics(t *testing.T) {
	type record struct {
		sample     bool
		MetricsKey sampler.MetricsKey
	}
	duplicate := func(n int, r record) []record {
		records := make([]record, n)
		for i := 0; i < n; i++ {
			records[i] = r
		}
		return records
	}
	t.Run("report-after-stop", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		statsdClient := mockStatsd.NewMockClientInterface(ctrl)
		metrics := sampler.NewMetrics(statsdClient)
		metrics.Add(sampler.NewPrioritySampler(&config.AgentConfig{}, &sampler.DynamicConfig{}))
		statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, float64(1)).Times(1)
		metrics.Start()
		metrics.Stop()
	})
	t.Run("additional-metrics", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		statsdClient := mockStatsd.NewMockClientInterface(ctrl)
		metrics := sampler.NewMetrics(statsdClient)
		metrics.Add(
			sampler.NewPrioritySampler(&config.AgentConfig{}, &sampler.DynamicConfig{}),
			sampler.NewNoPrioritySampler(&config.AgentConfig{}),
			sampler.NewErrorsSampler(&config.AgentConfig{}),
			sampler.NewRareSampler(&config.AgentConfig{}),
		)
		statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:priority"}, float64(1)).Times(1)
		statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:no_priority"}, float64(1)).Times(1)
		statsdClient.EXPECT().Gauge(sampler.MetricSamplerSize, float64(0), []string{"sampler:error"}, float64(1)).Times(1)
		statsdClient.EXPECT().Count(sampler.MetricsRareHits, int64(0), nil, float64(1)).Times(1)
		statsdClient.EXPECT().Count(sampler.MetricsRareMisses, int64(0), nil, float64(1)).Times(1)
		statsdClient.EXPECT().Gauge(sampler.MetricsRareShrinks, float64(0), nil, float64(1)).Times(1)
		metrics.Report()
	})
	t.Run("reset-metrics", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		statsdClient := mockStatsd.NewMockClientInterface(ctrl)
		metrics := sampler.NewMetrics(statsdClient)

		statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(3), []string{"sampler:rare", "target_service:service-1", "target_env:env-3"}, float64(1)).Times(1)
		statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(3), []string{"sampler:rare", "target_service:service-1", "target_env:env-3"}, float64(1)).Times(1)
		for _, r := range duplicate(3, record{sample: true, MetricsKey: sampler.NewMetricsKey("service-1", "env-3", sampler.NameRare, sampler.PriorityAutoDrop)}) {
			metrics.RecordSample(r.sample, r.MetricsKey)
		}
		metrics.Report()

		// Ensure that the metrics are reset.
		statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		statsdClient.EXPECT().Count(sampler.MetricSamplerKept, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		metrics.Report()

		statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(3), []string{"sampler:no_priority", "target_service:service-1", "target_env:env-3"}, float64(1)).Times(1)
		statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(3), []string{"sampler:no_priority", "target_service:service-1", "target_env:env-3"}, float64(1)).Times(1)
		for _, r := range duplicate(3, record{sample: true, MetricsKey: sampler.NewMetricsKey("service-1", "env-3", sampler.NameNoPriority, sampler.PriorityUserDrop)}) {
			metrics.RecordSample(r.sample, r.MetricsKey)
		}
		metrics.Report()

		// Ensure that the metrics are reset.
		statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		statsdClient.EXPECT().Count(sampler.MetricSamplerKept, gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		metrics.Report()
	})
	t.Run("concurrent-all-samplers", func(t *testing.T) {
		// To randomize the order of the records, we use a map.
		recordMap := map[string][][]record{
			"priority-auto-keep": {
				duplicate(11, record{sample: true, MetricsKey: sampler.NewMetricsKey("service-1", "env-1", sampler.NamePriority, sampler.PriorityAutoKeep)}),
				duplicate(12, record{sample: true, MetricsKey: sampler.NewMetricsKey("service-1", "", sampler.NamePriority, sampler.PriorityAutoKeep)}),
				duplicate(13, record{sample: true, MetricsKey: sampler.NewMetricsKey("service-1", "env-2", sampler.NamePriority, sampler.PriorityAutoKeep)}),
				duplicate(14, record{sample: true, MetricsKey: sampler.NewMetricsKey("service-2", "env-1", sampler.NamePriority, sampler.PriorityAutoKeep)}),
				duplicate(15, record{sample: true, MetricsKey: sampler.NewMetricsKey("", "env-1", sampler.NamePriority, sampler.PriorityAutoKeep)}),
			},
			"priority-auto-drop": {
				duplicate(21, record{sample: false, MetricsKey: sampler.NewMetricsKey("service-1", "env-1", sampler.NamePriority, sampler.PriorityAutoDrop)}),
				duplicate(22, record{sample: false, MetricsKey: sampler.NewMetricsKey("service-1", "", sampler.NamePriority, sampler.PriorityAutoDrop)}),
				duplicate(23, record{sample: false, MetricsKey: sampler.NewMetricsKey("service-1", "env-2", sampler.NamePriority, sampler.PriorityAutoDrop)}),
				duplicate(24, record{sample: false, MetricsKey: sampler.NewMetricsKey("service-2", "env-1", sampler.NamePriority, sampler.PriorityAutoDrop)}),
			},
			"priority-manual-keep": {
				duplicate(31, record{sample: true, MetricsKey: sampler.NewMetricsKey("service-1", "env-1", sampler.NamePriority, sampler.PriorityUserKeep)}),
				duplicate(32, record{sample: true, MetricsKey: sampler.NewMetricsKey("service-1", "", sampler.NamePriority, sampler.PriorityUserKeep)}),
				duplicate(33, record{sample: true, MetricsKey: sampler.NewMetricsKey("service-1", "env-2", sampler.NamePriority, sampler.PriorityUserKeep)}),
				duplicate(34, record{sample: true, MetricsKey: sampler.NewMetricsKey("service-2", "env-1", sampler.NamePriority, sampler.PriorityUserKeep)}),
			},
			"priority-manual-drop": {
				duplicate(41, record{sample: false, MetricsKey: sampler.NewMetricsKey("service-1", "env-1", sampler.NamePriority, sampler.PriorityUserDrop)}),
				duplicate(42, record{sample: false, MetricsKey: sampler.NewMetricsKey("service-1", "", sampler.NamePriority, sampler.PriorityUserDrop)}),
				duplicate(43, record{sample: false, MetricsKey: sampler.NewMetricsKey("service-1", "env-2", sampler.NamePriority, sampler.PriorityUserDrop)}),
				duplicate(44, record{sample: false, MetricsKey: sampler.NewMetricsKey("service-2", "env-1", sampler.NamePriority, sampler.PriorityUserDrop)}),
			},
			"nopriority": {
				duplicate(51, record{sample: true, MetricsKey: sampler.NewMetricsKey("service-1", "env-2", sampler.NameNoPriority, sampler.PriorityNone)}),
				duplicate(52, record{sample: false, MetricsKey: sampler.NewMetricsKey("service-2", "env-1", sampler.NameNoPriority, sampler.PriorityNone)}),
			},
			"error": {
				duplicate(61, record{sample: true, MetricsKey: sampler.NewMetricsKey("service-3", "env-3", sampler.NameError, sampler.PriorityNone)}),
				duplicate(62, record{sample: false, MetricsKey: sampler.NewMetricsKey("service-3", "", sampler.NameError, sampler.PriorityNone)}),
			},
			"probabilistic": {
				duplicate(71, record{sample: true, MetricsKey: sampler.NewMetricsKey("service-1", "env-2", sampler.NameProbabilistic, sampler.PriorityNone)}),
				duplicate(72, record{sample: false, MetricsKey: sampler.NewMetricsKey("service-4", "env-2", sampler.NameProbabilistic, sampler.PriorityNone)}),
			},
			"rare": {
				duplicate(1, record{sample: true, MetricsKey: sampler.NewMetricsKey("service-1", "env-3", sampler.NameRare, sampler.PriorityNone)}),
				duplicate(2, record{sample: false, MetricsKey: sampler.NewMetricsKey("", "", sampler.NameRare, sampler.PriorityNone)}),
			},
		}
		expectations := func(statsdClient *mockStatsd.MockClientInterface) {
			// priority-auto-keep
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(11), []string{"sampler:priority", "sampling_priority:auto_keep", "target_service:service-1", "target_env:env-1"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(11), []string{"sampler:priority", "sampling_priority:auto_keep", "target_service:service-1", "target_env:env-1"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(12), []string{"sampler:priority", "sampling_priority:auto_keep", "target_service:service-1"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(12), []string{"sampler:priority", "sampling_priority:auto_keep", "target_service:service-1"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(13), []string{"sampler:priority", "sampling_priority:auto_keep", "target_service:service-1", "target_env:env-2"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(13), []string{"sampler:priority", "sampling_priority:auto_keep", "target_service:service-1", "target_env:env-2"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(14), []string{"sampler:priority", "sampling_priority:auto_keep", "target_service:service-2", "target_env:env-1"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(14), []string{"sampler:priority", "sampling_priority:auto_keep", "target_service:service-2", "target_env:env-1"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(15), []string{"sampler:priority", "sampling_priority:auto_keep", "target_env:env-1"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(15), []string{"sampler:priority", "sampling_priority:auto_keep", "target_env:env-1"}, float64(1)).Times(1)
			// priority-auto-drop
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(21), []string{"sampler:priority", "sampling_priority:auto_drop", "target_service:service-1", "target_env:env-1"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(22), []string{"sampler:priority", "sampling_priority:auto_drop", "target_service:service-1"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(23), []string{"sampler:priority", "sampling_priority:auto_drop", "target_service:service-1", "target_env:env-2"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(24), []string{"sampler:priority", "sampling_priority:auto_drop", "target_service:service-2", "target_env:env-1"}, float64(1)).Times(1)
			// priority-manual-keep
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(31), []string{"sampler:priority", "sampling_priority:manual_keep", "target_service:service-1", "target_env:env-1"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(31), []string{"sampler:priority", "sampling_priority:manual_keep", "target_service:service-1", "target_env:env-1"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(32), []string{"sampler:priority", "sampling_priority:manual_keep", "target_service:service-1"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(32), []string{"sampler:priority", "sampling_priority:manual_keep", "target_service:service-1"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(33), []string{"sampler:priority", "sampling_priority:manual_keep", "target_service:service-1", "target_env:env-2"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(33), []string{"sampler:priority", "sampling_priority:manual_keep", "target_service:service-1", "target_env:env-2"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(34), []string{"sampler:priority", "sampling_priority:manual_keep", "target_service:service-2", "target_env:env-1"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(34), []string{"sampler:priority", "sampling_priority:manual_keep", "target_service:service-2", "target_env:env-1"}, float64(1)).Times(1)
			// priority-manual-drop
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(41), []string{"sampler:priority", "sampling_priority:manual_drop", "target_service:service-1", "target_env:env-1"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(42), []string{"sampler:priority", "sampling_priority:manual_drop", "target_service:service-1"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(43), []string{"sampler:priority", "sampling_priority:manual_drop", "target_service:service-1", "target_env:env-2"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(44), []string{"sampler:priority", "sampling_priority:manual_drop", "target_service:service-2", "target_env:env-1"}, float64(1)).Times(1)
			// nopriority
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(51), []string{"sampler:no_priority", "target_service:service-1", "target_env:env-2"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(51), []string{"sampler:no_priority", "target_service:service-1", "target_env:env-2"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(52), []string{"sampler:no_priority", "target_service:service-2", "target_env:env-1"}, float64(1)).Times(1)
			// error
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(61), []string{"sampler:error", "target_service:service-3", "target_env:env-3"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(61), []string{"sampler:error", "target_service:service-3", "target_env:env-3"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(62), []string{"sampler:error", "target_service:service-3"}, float64(1)).Times(1)
			// probabilistic
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(71), []string{"sampler:probabilistic", "target_service:service-1"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(71), []string{"sampler:probabilistic", "target_service:service-1"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(72), []string{"sampler:probabilistic", "target_service:service-4"}, float64(1)).Times(1)
			// rare
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(1), []string{"sampler:rare", "target_service:service-1", "target_env:env-3"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerKept, int64(1), []string{"sampler:rare", "target_service:service-1", "target_env:env-3"}, float64(1)).Times(1)
			statsdClient.EXPECT().Count(sampler.MetricSamplerSeen, int64(2), []string{"sampler:rare"}, float64(1)).Times(1)
		}
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		statsdClient := mockStatsd.NewMockClientInterface(ctrl)
		metrics := sampler.NewMetrics(statsdClient)
		expectations(statsdClient)
		var wg sync.WaitGroup
		for _, recordList := range recordMap {
			for _, records := range recordList {
				wg.Add(1)
				go func(records []record) {
					defer wg.Done()
					for _, r := range records {
						metrics.RecordSample(r.sample, r.MetricsKey)
					}
				}(records)
			}
		}
		wg.Wait()
		metrics.Report()
	})
}
