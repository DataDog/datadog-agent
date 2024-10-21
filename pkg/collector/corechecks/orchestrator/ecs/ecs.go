// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

// Package ecs is used for the orchestrator ECS check
package ecs

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/atomic"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/ecs"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// CheckName is the name of the check
const CheckName = "orchestrator_ecs"

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
	sender                     sender.Sender
	config                     *oconfig.OrchestratorConfig
	collectors                 []collectors.Collector
	groupID                    *atomic.Int32
	workloadmetaStore          workloadmeta.Component
	tagger                     tagger.Component
	isECSCollectionEnabledFunc func() bool
	awsAccountID               int
	clusterName                string
	region                     string
	clusterID                  string
	hostName                   string
	systemInfo                 *model.SystemInfo
}

// Factory creates a new check factory
func Factory(store workloadmeta.Component, tagger tagger.Component) optional.Option[func() check.Check] {
	return optional.NewOption(func() check.Check { return newCheck(store, tagger) })
}

func newCheck(store workloadmeta.Component, tagger tagger.Component) check.Check {
	return &Check{
		CheckBase:                  core.NewCheckBase(CheckName),
		workloadmetaStore:          store,
		tagger:                     tagger,
		config:                     oconfig.NewDefaultOrchestratorConfig(),
		groupID:                    atomic.NewInt32(rand.Int31()),
		isECSCollectionEnabledFunc: oconfig.IsOrchestratorECSExplorerEnabled,
	}
}

// Configure the Orchestrator ECS check
// nil check to allow for overrides
func (c *Check) Configure(
	senderManager sender.SenderManager,
	integrationConfigDigest uint64,
	data integration.Data,
	initConfig integration.Data,
	source string,
) error {
	c.BuildID(integrationConfigDigest, data, initConfig)

	err := c.CommonConfigure(senderManager, initConfig, data, source)
	if err != nil {
		return err
	}

	err = c.config.Load()
	if err != nil {
		return err
	}

	if c.isECSCollectionEnabledFunc == nil {
		c.isECSCollectionEnabledFunc = oconfig.IsOrchestratorECSExplorerEnabled
	}

	if !c.isECSCollectionEnabledFunc() {
		log.Debug("Orchestrator ECS Collection is disabled")
		return nil
	}

	if c.sender == nil {
		sender, err := c.GetSender()
		if err != nil {
			return err
		}
		c.sender = sender
	}

	c.systemInfo, err = checks.CollectSystemInfo()
	if err != nil {
		log.Warnf("Failed to collect system info: %s", err)
	}

	c.hostName, _ = hostname.Get(context.TODO())

	return nil
}

// Run executes the check
func (c *Check) Run() error {
	if !c.shouldRun() {
		return nil
	}

	c.initCollectors()

	for _, collector := range c.collectors {
		if collector.Metadata().IsSkipped {
			c.Warnf("collector %s is skipped: %s", collector.Metadata().Name, collector.Metadata().SkippedReason)
			continue
		}

		runStartTime := time.Now()
		runConfig := &collectors.CollectorRunConfig{
			ECSCollectorRunConfig: collectors.ECSCollectorRunConfig{
				WorkloadmetaStore: c.workloadmetaStore,
				AWSAccountID:      c.awsAccountID,
				Region:            c.region,
				ClusterName:       c.clusterName,
				HostName:          c.hostName,
				SystemInfo:        c.systemInfo,
			},
			Config:      c.config,
			MsgGroupRef: c.groupID,
			ClusterID:   c.clusterID,
		}
		result, err := collector.Run(runConfig)
		if err != nil {
			_ = c.Warnf("ECSCollector %s failed to run: %s", collector.Metadata().FullName(), err.Error())
			continue
		}
		runDuration := time.Since(runStartTime)
		log.Debugf("ECSCollector %s run stats: listed=%d processed=%d messages=%d duration=%s", collector.Metadata().FullName(), result.ResourcesListed, result.ResourcesProcessed, len(result.Result.MetadataMessages), runDuration)

		c.sender.OrchestratorMetadata(result.Result.MetadataMessages, runConfig.ClusterID, int(collector.Metadata().NodeType))
	}
	return nil
}

func (c *Check) shouldRun() bool {
	if c.isECSCollectionEnabledFunc == nil || !c.isECSCollectionEnabledFunc() {
		log.Debug("Orchestrator ECS Collection is disabled")
		return false
	}

	c.initConfig()

	if c.region == "" || c.awsAccountID == 0 || c.clusterName == "" || c.clusterID == "" {
		log.Warnf("Orchestrator ECS check is missing required information, region: %s, awsAccountID: %d, clusterName: %s, clusterID: %s", c.region, c.awsAccountID, c.clusterName, c.clusterID)
		return false
	}
	return true
}

func (c *Check) initConfig() {
	if c.awsAccountID != 0 && c.region != "" && c.clusterName != "" && c.clusterID != "" {
		return
	}

	tasks := c.workloadmetaStore.ListECSTasks()
	if len(tasks) == 0 {
		return
	}

	c.awsAccountID = tasks[0].AWSAccountID
	c.clusterName = tasks[0].ClusterName
	c.region = tasks[0].Region

	if tasks[0].Region == "" || tasks[0].AWSAccountID == 0 {
		c.region, c.awsAccountID = getRegionAndAWSAccountID(tasks[0].EntityID.ID)
	}

	c.clusterID = initClusterID(c.awsAccountID, c.region, tasks[0].ClusterName)
}

func (c *Check) initCollectors() {
	c.collectors = []collectors.Collector{ecs.NewTaskCollector(c.tagger)}
}

// initClusterID generates a cluster ID from the AWS account ID, region and cluster name.
func initClusterID(awsAccountID int, region, clusterName string) string {
	cluster := fmt.Sprintf("%d/%s/%s", awsAccountID, region, clusterName)

	hash := md5.New()
	hash.Write([]byte(cluster))
	hashString := hex.EncodeToString(hash.Sum(nil))
	uuid, err := uuid.FromBytes([]byte(hashString[0:16]))
	if err != nil {
		log.Errorc(err.Error(), orchestrator.ExtraLogContext...)
		return ""
	}
	return uuid.String()
}

// ParseRegionAndAWSAccountID parses the region and AWS account ID from an ARN.
// https://docs.aws.amazon.com/IAM/latest/UserGuide/reference-arns.html#arns-syntax
func getRegionAndAWSAccountID(arn string) (string, int) {
	arnParts := strings.Split(arn, ":")
	if len(arnParts) < 5 {
		return "", 0
	}
	if arnParts[0] != "arn" || strings.Index(arnParts[1], "aws") != 0 {
		return "", 0
	}
	region := arnParts[3]
	if strings.Count(region, "-") < 2 {
		region = ""
	}

	id := arnParts[4]
	// aws account id is 12 digits
	// https://docs.aws.amazon.com/accounts/latest/reference/manage-acct-identifiers.html
	if len(id) != 12 {
		return region, 0
	}
	awsAccountID, err := strconv.Atoi(id)
	if err != nil {
		return region, 0
	}

	return region, awsAccountID
}
