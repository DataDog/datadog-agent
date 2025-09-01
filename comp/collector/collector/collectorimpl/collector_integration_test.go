// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectorimpl

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core"
	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfx "github.com/DataDog/datadog-agent/comp/core/tagger/fx-noop"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestSchedulerSpam(t *testing.T) {
	tempDir := t.TempDir()
	checkDir := tempDir
	checkName := "testcheck"
	os.MkdirAll(checkDir, 0755)
	os.WriteFile(filepath.Join(checkDir, fmt.Sprintf("%s.py", checkName)), []byte(PythonCheck), 0644)

	deps := fxutil.Test[dependencies](t,
		core.MockBundle(),
		demultiplexerimpl.MockModule(),
		haagentmock.Module(),
		fx.Provide(func() option.Option[serializer.MetricSerializer] {
			return option.None[serializer.MetricSerializer]()
		}),
		fx.Provide(func() option.Option[agenttelemetry.Component] {
			return option.None[agenttelemetry.Component]()
		}),
		fx.Replace(config.MockParams{
			Overrides: map[string]interface{}{
				"additional_checksd":   tempDir,
				"check_cancel_timeout": 100 * time.Millisecond,
				// to ease debugging
				"c_core_dump":             true,
				"c_stacktrace_collection": true,
			},
		}),
	)

	tagger := fxutil.Test[tagger.Component](t, taggerfx.Module())

	var coll collector.Component = newCollector(deps)
	checkScheduler := pkgcollector.InitCheckScheduler(option.New(coll), deps.SenderManager, option.None[integrations.Component](), tagger)

	log.Infof("Starting scheduler spam")

	for i := 0; i < 10; i++ {
		checkScheduler.Schedule([]integration.Config{
			{
				Name:       checkName,
				InitConfig: []byte("{}"),
				Instances: []integration.Data{
					[]byte(fmt.Sprintf(CheckConfigTemplate, i)),
				},
			},
		})
	}

	// this test doesn't test much for now so it can succeed but actually show failures in logs
	t.Fatal("Force showing logs in CI")
}

const CheckConfigTemplate = `{"id": %d}`

const PythonCheck = `
from datadog_checks.base import AgentCheck

class HTTPCheck(AgentCheck):
	def check(self, instance):
		checkId = instance.get('id')
		print(f"Running check {checkId}")
`
