// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package process implements the local process collector for
// Workloadmeta.
package process

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	processwlm "github.com/DataDog/datadog-agent/pkg/process/metadata/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestProcessCollector(t *testing.T) {
	originalFlavor := flavor.GetFlavor()
	defer flavor.SetFlavor(originalFlavor)
	flavor.SetFlavor(flavor.DefaultAgent)

	configOverrides := map[string]interface{}{
		"language_detection.enabled": true,
	}

	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Replace(config.MockParams{Overrides: configOverrides}),
		fx.Supply(workloadmeta.Params{
			AgentType: workloadmeta.NodeAgent,
		}),
		workloadmetafxmock.MockModuleV2(),
	))

	time.Sleep(time.Second)

	processDiffCh := make(chan *processwlm.ProcessCacheDiff)
	processCollector := &collector{
		id:            collectorID,
		catalog:       workloadmeta.NodeAgent,
		processDiffCh: processDiffCh,
		store:         mockStore,
	}

	ctx, cancel := context.WithCancel(context.TODO())
	go processCollector.stream(ctx)

	creationTime := time.Now().Unix()
	processDiffCh <- &processwlm.ProcessCacheDiff{
		Creation: []*processwlm.ProcessEntity{
			{
				Pid:          1,
				ContainerId:  "cid",
				NsPid:        1,
				CreationTime: creationTime,
				Language:     &languagemodels.Language{Name: languagemodels.Java},
			},
		},
	}

	expectedProc1 := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			ID:   "1",
			Kind: workloadmeta.KindProcess,
		},
		NsPid:        1,
		ContainerID:  "cid",
		CreationTime: time.UnixMilli(creationTime),
		Language:     &languagemodels.Language{Name: languagemodels.Java},
	}

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		proc, err := mockStore.GetProcess(1)
		assert.NoError(c, err)
		assert.Equal(c, expectedProc1, proc)
	}, time.Second, time.Millisecond*100)

	processDiffCh <- &processwlm.ProcessCacheDiff{
		Creation: []*processwlm.ProcessEntity{
			{
				Pid:          2,
				ContainerId:  "cid",
				NsPid:        2,
				CreationTime: creationTime,
				Language:     &languagemodels.Language{Name: languagemodels.Python},
			},
		},
		Deletion: []*processwlm.ProcessEntity{
			{
				Pid:          1,
				ContainerId:  "cid",
				NsPid:        1,
				CreationTime: creationTime,
				Language:     &languagemodels.Language{Name: languagemodels.Java},
			},
		},
	}

	expectedProc2 := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			ID:   "2",
			Kind: workloadmeta.KindProcess,
		},
		NsPid:        2,
		ContainerID:  "cid",
		CreationTime: time.UnixMilli(creationTime),
		Language:     &languagemodels.Language{Name: languagemodels.Python},
	}

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		proc, err := mockStore.GetProcess(2)
		assert.NoError(c, err)
		assert.Equal(c, expectedProc2, proc)

		_, err = mockStore.GetProcess(1)
		assert.Error(c, err)
	}, time.Second, time.Millisecond*100)

	cancel()
}

func TestProcessCollectorStart(t *testing.T) {
	tests := []struct {
		name                      string
		agentFlavor               string
		langDetectionEnabled      bool
		processesRunInCoreEnabled bool
		expectedEnabled           bool
	}{
		{
			name:                      "core agent + configs enabled",
			agentFlavor:               flavor.DefaultAgent,
			langDetectionEnabled:      true,
			processesRunInCoreEnabled: true,
			expectedEnabled:           true,
		},
		{
			name:                      "core agent + checks disabled + lang detection",
			agentFlavor:               flavor.DefaultAgent,
			langDetectionEnabled:      true,
			processesRunInCoreEnabled: false,
			expectedEnabled:           false,
		},
		{
			name:                      "core agent + configs disabled",
			agentFlavor:               flavor.DefaultAgent,
			langDetectionEnabled:      false,
			processesRunInCoreEnabled: false,
			expectedEnabled:           false,
		},
		{
			name:                      "process agent + configs enabled",
			agentFlavor:               flavor.ProcessAgent,
			langDetectionEnabled:      true,
			processesRunInCoreEnabled: true,
			expectedEnabled:           false,
		},
		{
			name:                      "process agent + configs disabled",
			agentFlavor:               flavor.ProcessAgent,
			langDetectionEnabled:      false,
			processesRunInCoreEnabled: false,
			expectedEnabled:           false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			originalFlavor := flavor.GetFlavor()
			defer flavor.SetFlavor(originalFlavor)
			flavor.SetFlavor(test.agentFlavor)

			configOverrides := map[string]interface{}{
				"language_detection.enabled":                test.langDetectionEnabled,
				"process_config.process_collection.enabled": test.processesRunInCoreEnabled,
				"process_config.run_in_core_agent.enabled":  test.processesRunInCoreEnabled,
			}

			mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
				core.MockBundle(),
				fx.Replace(config.MockParams{Overrides: configOverrides}),
				fx.Supply(workloadmeta.Params{
					AgentType: workloadmeta.NodeAgent,
				}),
				workloadmetafxmock.MockModuleV2(),
			))

			processCollector := &collector{
				id:      collectorID,
				catalog: workloadmeta.NodeAgent,
			}

			ctx, cancel := context.WithCancel(context.TODO())

			err := processCollector.Start(ctx, mockStore)
			enabled := err == nil
			assert.Equal(t, test.expectedEnabled, enabled)

			cancel()
		})
	}
}
