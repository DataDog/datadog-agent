package networkpath

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/pkg/errors"
)

const CheckName = "networkpath"

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
	config        *CheckConfig
	lastCheckTime time.Time
}

// Program constants and default values
const (
	DefaultSourcePort   = 12345
	DefaultDestPort     = 33434
	DefaultNumPaths     = 10
	DefaultMinTTL       = 1
	DefaultMaxTTL       = 30
	DefaultDelay        = 50 //msec
	DefaultReadTimeout  = 3 * time.Second
	DefaultOutputFormat = "json"
)

// Run executes the check
func (c *Check) Run() error {
	startTime := time.Now()
	senderInstance, err := c.GetSender()
	if err != nil {
		return err
	}

	cfg := traceroute.Config{
		DestHostname: c.config.DestHostname,
	}

	trcrt := traceroute.New(cfg)
	path, err := trcrt.Run()
	if err != nil {
		return errors.Errorf("failed to trace path: %w", err)
	}

	// send to EP
	err = c.SendNetPathMDToEP(senderInstance, path)
	if err != nil {
		log.Errorf("failed to send network path metadata: %w", err)
	}

	tags := []string{
		"dest_hostname:" + c.config.DestHostname,
		"dest_name:" + c.config.DestName,
	}
	duration := time.Since(startTime)
	senderInstance.Gauge("networkpath.telemetry.count", 1, "", tags)
	senderInstance.Gauge("networkpath.telemetry.duration", duration.Seconds(), "", tags)

	if !c.lastCheckTime.IsZero() {
		interval := startTime.Sub(c.lastCheckTime)
		senderInstance.Gauge("networkpath.telemetry.interval", interval.Seconds(), "", tags)
	}
	senderInstance.Commit()

	numWorkers := config.Datadog.GetInt("check_runners")
	senderInstance.Gauge("networkpath.telemetry.check_runners", float64(numWorkers), "", tags)
	senderInstance.Gauge("networkpath.telemetry.fake_event_multiplier", float64(c.config.FakeEventMultiplier), "", tags)
	senderInstance.Gauge("networkpath.telemetry.hop_count", float64(len(path.Hops)), "", tags)
	c.lastCheckTime = startTime

	senderInstance.Commit()
	return nil
}

func (c *Check) SendNetPathMDToEP(sender sender.Sender, path traceroute.NetworkPath) error {
	payloadBytes, err := json.Marshal(path)
	if err != nil {
		return fmt.Errorf("error marshalling device metadata: %s", err)
	}
	log.Debugf("traceroute path metadata payload: %s", string(payloadBytes))
	sender.EventPlatformEvent(payloadBytes, eventplatform.EventTypeNetworkPath)
	return nil
}

// Configure the networkpath check
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	err := c.CommonConfigure(senderManager, integrationConfigDigest, rawInitConfig, rawInstance, source)
	if err != nil {
		return fmt.Errorf("common configure failed: %s", err)
	}

	// Must be called before c.CommonConfigure
	c.BuildID(integrationConfigDigest, rawInstance, rawInitConfig)

	config, err := NewCheckConfig(rawInstance, rawInitConfig)
	if err != nil {
		return err
	}
	c.config = config
	return nil
}

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	return optional.NewOption(newCheck)
}

func newCheck() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(CheckName),
	}
}
