package containers

import (
	"fmt"
	"math"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

// DockerCheck grabs docker metrics
type DockerCheck struct {
	lastWarnings []error
}

// Run executes the check
func (d *DockerCheck) Run() error {
	log.Error("Running the docker go check !!!!")
	sender, err := aggregator.GetSender(d.ID())

	containers, err := docker.AllContainers()
	if err != nil {
		return fmt.Errorf("Could not list containers: %s", err)
	}

	for _, c := range containers {
		tags := []string{fmt.Sprintf("image:%s", c.Image), fmt.Sprintf("image_id:%s", c.ImageID)}
		sender.Rate("docker.cpu.system", float64(c.CPU.System), "", tags)
		sender.Rate("docker.cpu.user", float64(c.CPU.User), "", tags)
		sender.Rate("docker.cpu.usage", c.CPU.UsageTotal, "", tags)
		sender.Rate("docker.cpu.throttled", float64(c.CPUNrThrottled), "", tags)
		sender.Gauge("docker.mem.cache", float64(c.Memory.Cache), "", tags)
		sender.Gauge("docker.mem.rss", float64(c.Memory.RSS), "", tags)
		sender.Gauge("docker.mem.swap", float64(c.Memory.Swap), "", tags)

		if c.Memory.HierarchicalMemoryLimit < uint64(math.Pow(2, 60)) {
			sender.Gauge("docker.mem.limit", float64(c.Memory.HierarchicalMemoryLimit), "", tags)
			if c.Memory.HierarchicalMemoryLimit != 0 {
				sender.Gauge("docker.mem.in_use", float64(c.Memory.RSS/c.Memory.HierarchicalMemoryLimit), "", tags)
			}
		}

		if c.Memory.HierarchicalMemSWLimit < uint64(math.Pow(2, 60)) {
			sender.Gauge("docker.mem.sw_limit", float64(c.Memory.HierarchicalMemSWLimit), "", tags)
			if c.Memory.HierarchicalMemSWLimit != 0 {
				sender.Gauge("docker.mem.sw_in_use",
					float64((c.Memory.Swap+c.Memory.RSS)/c.Memory.HierarchicalMemSWLimit), "", tags)
			}
		}

		sender.Rate("docker.io.read_bytes", float64(c.IO.ReadBytes), "", tags)
		sender.Rate("docker.io.write_bytes", float64(c.IO.WriteBytes), "", tags)
	}
	sender.Commit()
	return nil
}

// Stop does nothing
func (d *DockerCheck) Stop() {}

func (d *DockerCheck) String() string {
	return "docker"
}

func (d *DockerCheck) Configure(config, initConfig check.ConfigData) error {
	docker.InitDockerUtil(&docker.Config{
		CacheDuration:  10 * time.Second,
		CollectNetwork: false,
	})
	return nil
}

// Interval returns the scheduling time for the check
func (d *DockerCheck) Interval() time.Duration {
	return check.DefaultCheckInterval
}

// ID returns the name of the check since there should be only one instance running
func (d *DockerCheck) ID() check.ID {
	return check.ID(d.String())
}

// GetWarnings grabs the last warnings from the sender
func (d *DockerCheck) GetWarnings() []error {
	w := d.lastWarnings
	d.lastWarnings = []error{}
	return w
}

// GetMetricStats returns the stats from the last run of the check
func (d *DockerCheck) GetMetricStats() (map[string]int64, error) {
	sender, err := aggregator.GetSender(d.ID())
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve a Sender instance: %v", err)
	}
	return sender.GetMetricStats(), nil
}

func dockerFactory() check.Check {
	return &DockerCheck{}
}

func init() {
	core.RegisterCheck("docker", dockerFactory)
}
