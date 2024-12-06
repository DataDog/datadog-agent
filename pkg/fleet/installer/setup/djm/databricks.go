// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package djm contains data-jobs-monitoring installation logic
package djm

import (
	"context"
	"os"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
	databricksInjectorVersion = "0.21.0"
	databricksJavaVersion     = "1.41.1"
	databricksAgentVersion    = "7.57.2"
	logsService               = "databricks"
)

type databricksSetup struct {
	ctx context.Context
	*common.HostInstaller
	setupIssues []string
}

// SetupDatabricks sets up the Databricks environment
func SetupDatabricks(ctx context.Context, env *env.Env) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "setup.databricks")
	defer func() { span.Finish(tracer.WithError(err)) }()

	i, err := common.NewHostInstaller(env)
	if err != nil {
		return err
	}
	ds := &databricksSetup{
		ctx:           ctx,
		HostInstaller: i,
	}
	return ds.setup()
}

func (ds *databricksSetup) setup() error {
	// agent binary to install
	ds.SetAgentVersion(databricksAgentVersion)

	// avoid port conflict
	ds.AddAgentConfig("process_config.expvar_port", -1)
	ds.AddAgentConfig("expected_tags_duration", "10m")
	ds.AddAgentConfig("djm_config.enabled", true)

	ds.extractHostTagsFromEnv()

	span, _ := tracer.SpanFromContext(ds.ctx)
	switch os.Getenv("DB_IS_DRIVER") {
	case "true":
		span.SetTag("spark_node", "driver")
		return ds.setupDatabricksDriver()
	default:
		span.SetTag("spark_node", "worker")
		return ds.setupDatabricksExecutor()
	}
}

type varExtraction struct {
	envVar string
	tagKey string
}

var varExtractions = []varExtraction{
	{"DATABRICKS_WORKSPACE", "workspace"},
	{"DB_CLUSTER_NAME", "databricks_cluster_name"},
	{"DB_CLUSTER_ID", "databricks_cluster_id"},
	{"DB_NODE_TYPE", "databricks_node_type"},
}

func (ds *databricksSetup) extractHostTagsFromEnv() {
	for _, ve := range varExtractions {
		if val, ok := os.LookupEnv(ve.envVar); ok {
			ds.AddHostTag(ve.tagKey, val)
			continue
		}
		ds.setupIssues = append(ds.setupIssues, ve.envVar+"_not_set")
	}
}

func (ds *databricksSetup) setupDatabricksDriver() error {
	ds.AddHostTag("node_type", "driver")

	ds.driverLogCollection()

	ds.setupAgentSparkCheck()

	ds.AddTracerEnv("DD_DATA_JOBS_ENABLED", "true")
	ds.AddTracerEnv("DD_INTEGRATIONS_ENABLED", "false")

	// APM binaries to install
	ds.SetInjectorVersion(databricksInjectorVersion)
	ds.SetJavaTracerVersion(databricksJavaVersion)

	return ds.ConfigureAndInstall(ds.ctx)
}

func (ds *databricksSetup) setupDatabricksExecutor() error {
	ds.AddHostTag("node_type", "worker")
	ds.workerLogCollection()
	return ds.ConfigureAndInstall(ds.ctx)
}

func (ds *databricksSetup) setupAgentSparkCheck() {
	driverIP := os.Getenv("DB_DRIVER_IP")
	if driverIP == "" {
		log.Warn("DB_DRIVER_IP not set")
		return
	}
	driverPort := os.Getenv("DB_DRIVER_PORT")
	if driverPort == "" {
		log.Warn("DB_DRIVER_PORT not set")
		return
	}
	clusterName := os.Getenv("DB_CLUSTER_NAME")

	ds.AddSparkInstance(common.SparkInstance{
		SparkURL:         "http://" + driverIP + ":" + driverPort,
		SparkClusterMode: "spark_driver_mode",
		ClusterName:      clusterName,
		StreamingMetrics: true,
	})
}

func (ds *databricksSetup) driverLogCollection() {
	if os.Getenv("DRIVER_LOGS_ENABLED") != "true" {
		return
	}
	span, _ := tracer.SpanFromContext(ds.ctx)
	span.SetTag("driver_logs", "enabled")
	log.Info("Enabling logs collection on the driver")
	ds.AddLogConfig(common.LogConfig{
		Type:    "file",
		Path:    "/databricks/driver/logs/*.log",
		Source:  "driver_logs",
		Service: logsService,
	})
	ds.AddLogConfig(common.LogConfig{
		Type:    "file",
		Path:    "/databricks/driver/logs/stderr",
		Source:  "driver_stderr",
		Service: logsService,
	})
	ds.AddLogConfig(common.LogConfig{
		Type:    "file",
		Path:    "/databricks/driver/logs/stdout",
		Source:  "driver_stdout",
		Service: logsService,
	})
}

func (ds *databricksSetup) workerLogCollection() {
	if os.Getenv("WORKER_LOGS_ENABLED") != "true" {
		return
	}
	span, _ := tracer.SpanFromContext(ds.ctx)
	span.SetTag("worker_logs", "enabled")
	log.Info("Enabling logs collection on the executor")
	ds.AddLogConfig(common.LogConfig{
		Type:    "file",
		Path:    "/databricks/spark/work/*/*/*.log",
		Source:  "worker_logs",
		Service: logsService,
	})
	ds.AddLogConfig(common.LogConfig{
		Type:    "file",
		Path:    "/databricks/spark/work/*/*/stderr",
		Source:  "worker_stderr",
		Service: logsService,
	})
	ds.AddLogConfig(common.LogConfig{
		Type:    "file",
		Path:    "/databricks/spark/work/*/*/stdout",
		Source:  "worker_stdout",
		Service: logsService,
	})
}
