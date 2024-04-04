// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package docker

import (
	"context"
	"flag"
	"os"
	"strings"
	"testing"
	"time"

	log "github.com/cihub/seelog"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	compcfg "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/docker"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/test/integration/utils"
)

var (
	retryDelay   = flag.Int("retry-delay", 1, "time to wait between retries (default 1 second)")
	retryTimeout = flag.Int("retry-timeout", 30, "maximum time before failure (default 30 seconds)")
	skipCleanup  = flag.Bool("skip-cleanup", false, "skip cleanup of the docker containers (for debugging)")
)

var dockerCfgString = `
collect_events: true
collect_container_size: true
collect_images_stats: true
collect_exit_codes: true
tags:
  - instanceTag:MustBeHere
`

var datadogCfgString = `
docker_labels_as_tags:
    "high.card.label": +highcardlabeltag
    "low.card.label": lowcardlabeltag
docker_env_as_tags:
    "HIGH_CARD_ENV": +highcardenvtag
    "low_card_env": lowcardenvtag
`

var (
	sender      *mocksender.MockSender
	dockerCheck check.Check
	fxApp       *fx.App
)

func TestMain(m *testing.M) {
	flag.Parse()

	config.SetupLogger(
		config.LoggerName("test"),
		"debug",
		"",
		"",
		false,
		true,
		false,
	)

	retryTicker := time.NewTicker(time.Duration(*retryDelay) * time.Second)
	timeoutTicker := time.NewTicker(time.Duration(*retryTimeout) * time.Second)
	var lastRunResult int
	var retryCount int

	store, err := setup()
	if err != nil {
		log.Infof("Test setup failed: %v", err)
		tearOffAndExit(1)
	}

	for {
		select {
		case <-retryTicker.C:
			retryCount++
			log.Infof("Starting run %d", retryCount)
			lastRunResult = doRun(m, store)
			if lastRunResult == 0 {
				tearOffAndExit(0)
			}
		case <-timeoutTicker.C:
			log.Errorf("Timeout after %d seconds and %d retries", retryTimeout, retryCount)
			tearOffAndExit(1)
		}
	}
}

type testDeps struct {
	fx.In
	Store      workloadmeta.Component
	TaggerComp tagger.Component
}

// Called before for first test run: compose up
func setup() (workloadmeta.Component, error) {
	// Setup global conf
	config.Datadog.SetConfigType("yaml")
	err := config.Datadog.ReadConfig(strings.NewReader(datadogCfgString))
	if err != nil {
		return nil, err
	}
	config.SetFeaturesNoCleanup(config.Docker)

	// Note: workloadmeta will be started by fx with the App
	var deps testDeps
	fxApp, deps, err = fxutil.TestApp[testDeps](fx.Options(
		fx.Supply(compcfg.NewAgentParams(
			"", compcfg.WithConfigMissingOK(true))),
		compcfg.Module(),
		fx.Supply(logimpl.ForOneShot("TEST", "info", false)),
		logimpl.Module(),
		fx.Supply(workloadmeta.NewParams()),
		collectors.GetCatalog(),
		workloadmeta.Module(),
		tagger.Module(),
		fx.Supply(tagger.NewTaggerParams()),
	))
	store := deps.Store

	// Start compose recipes
	for projectName, file := range defaultCatalog.composeFilesByProjects {
		compose := &utils.ComposeConf{
			ProjectName: projectName,
			FilePath:    file,
		}
		output, err := compose.Start()
		if err != nil {
			log.Errorf("Compose didn't start properly: %s", string(output))
			return nil, err
		}
	}
	return store, nil
}

// Reset the state and trigger a new run
func doRun(m *testing.M, store workloadmeta.Component) int {
	factory := docker.Factory(store)
	checkFactory, _ := factory.Get()
	dockerCheck = checkFactory()

	// Setup mock sender
	sender = mocksender.NewMockSender(dockerCheck.ID())
	sender.SetupAcceptAll()

	// Setup docker check
	dockerCfg := integration.Data(dockerCfgString)
	dockerInitCfg := integration.Data("")
	dockerCheck.Configure(sender.GetSenderManager(), integration.FakeConfigHash, dockerCfg, dockerInitCfg, "test")

	dockerCheck.Run()
	return m.Run()
}

// Compose down, reset the external states and exit
func tearOffAndExit(exitcode int) {
	if *skipCleanup {
		os.Exit(exitcode)
	}

	_ = fxApp.Stop(context.TODO())

	// Stop compose recipes, ignore errors
	for projectName, file := range defaultCatalog.composeFilesByProjects {
		compose := &utils.ComposeConf{
			ProjectName: projectName,
			FilePath:    file,
		}
		output, err := compose.Stop()
		if err != nil {
			log.Warnf("Compose didn't stop properly: %s", string(output))
		}
	}
	os.Exit(exitcode)
}
