// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ciscosdwan implements NDM Cisco SD-WAN corecheck
package deviceconfig

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/snmp/utils"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	// CheckName is the name of the check
	CheckName            = "device_config"
	defaultCheckInterval = 5 * time.Minute
)

// Configuration for the Cisco SD-WAN check
type checkCfg struct {
	IPAddress             string `yaml:"ip_address"`
	Port                  string `yaml:"port"`
	Username              string `yaml:"username"`
	Password              string `yaml:"password"`
	Namespace             string `yaml:"namespace"`
	MinCollectionInterval int    `yaml:"min_collection_interval"`
}

// DeviceConfigCheck contains the field for the CiscoSdwanCheck
type DeviceConfigCheck struct {
	core.CheckBase
	interval time.Duration
	config   checkCfg
}

// Run executes the check
func (c *DeviceConfigCheck) Run() error {
	client, session, err := connectToHost(fmt.Sprintf("%s:%s", c.config.IPAddress, c.config.Port), c.config.Username, c.config.Password)
	if err != nil {
		return err
	}
	defer client.Close()

	out, err := session.CombinedOutput("show running-config")
	if err != nil {
		return err
	}

	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	config := string(out)
	fmt.Println(config)

	collectionTime := time.Now()

	payload := devicemetadata.NetworkDevicesMetadata{
		Namespace:        c.config.Namespace,
		CollectTimestamp: collectionTime.Unix(),
		Configs: []devicemetadata.Config{
			{
				DeviceID: fmt.Sprintf("%s:%s", c.config.Namespace, c.config.IPAddress),
				Value:    config,
			},
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	sender.EventPlatformEvent(payloadBytes, eventplatform.EventTypeNetworkDevicesMetadata)

	return nil
}

// Configure the Cisco SD-WAN check
func (c *DeviceConfigCheck) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	// Must be called before c.CommonConfigure
	c.BuildID(integrationConfigDigest, rawInstance, rawInitConfig)

	err := c.CommonConfigure(senderManager, rawInitConfig, rawInstance, source)
	if err != nil {
		return err
	}

	var instanceConfig checkCfg

	err = yaml.Unmarshal(rawInstance, &instanceConfig)
	if err != nil {
		return err
	}
	c.config = instanceConfig

	if c.config.Namespace == "" {
		c.config.Namespace = "default"
	} else {
		namespace, err := utils.NormalizeNamespace(c.config.Namespace)
		if err != nil {
			return err
		}
		c.config.Namespace = namespace
	}

	if c.config.MinCollectionInterval != 0 {
		c.interval = time.Second * time.Duration(c.config.MinCollectionInterval)
	}

	return nil
}

func connectToHost(host, user, pass string) (*ssh.Client, *ssh.Session, error) {
	sshConfig := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{ssh.Password(pass)},
	}
	sshConfig.HostKeyCallback = ssh.InsecureIgnoreHostKey()

	client, err := ssh.Dial("tcp", host, sshConfig)
	if err != nil {
		return nil, nil, err
	}

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, nil, err
	}

	return client, session, nil
}

// Interval returns the scheduling time for the check
func (c *DeviceConfigCheck) Interval() time.Duration {
	return c.interval
}

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	return optional.NewOption(newCheck)
}

func newCheck() check.Check {
	return &DeviceConfigCheck{
		CheckBase: core.NewCheckBase(CheckName),
		interval:  defaultCheckInterval,
	}
}
