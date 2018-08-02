package clusteragent

import (
	"encoding/json"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/render"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// GetStatus returns status info for the Datadog Cluster Agent.
func GetStatus() map[string]interface{} {
	status := make(map[string]interface{})
	status["forwarderStats"] = forwarder.GetStatus()
	status["runnerStats"] = collector.GetRunnerStatus()
	status["autoConfigStats"] = autodiscovery.GetAutoConfigStatus()
	status["checkSchedulerStats"] = collector.GetCheckSchedulerStatus()
	status["config"] = getPartialConfig()
	status["conf_file"] = config.Datadog.ConfigFileUsed()
	status["version"] = version.DCAVersion
	status["pid"] = os.Getpid()
	hostname, err := util.GetHostname()
	if err != nil {
		log.Errorf("Error grabbing hostname for status: %v", err)
		status["metadata"] = host.GetPayloadFromCache("unknown")
	} else {
		status["metadata"] = host.GetPayloadFromCache(hostname)
	}
	now := time.Now()
	status["time"] = now.Format(render.TimeFormat)
	status["leaderelection"] = leaderelection.GetStatus()
	status["custommetrics"] = custommetrics.GetStatus()
	return status
}

// getPartialConfig returns config parameters of interest for the status page.
func getPartialConfig() map[string]string {
	conf := make(map[string]string)
	conf["log_file"] = config.Datadog.GetString("log_file")
	conf["log_level"] = config.Datadog.GetString("log_level")
	conf["confd_path"] = config.Datadog.GetString("confd_path")
	return conf
}

// GetAndFormatStatus gets and formats the Datadog Cluster Agent status all in one go.
func GetAndFormatStatus() ([]byte, error) {
	status := GetStatus()
	statusJSON, err := json.Marshal(status)
	if err != nil {
		log.Infof("Error while marshalling %q", err)
		return nil, err
	}
	st, err := FormatStatus(statusJSON)
	if err != nil {
		log.Infof("Error formatting the status %q", err)
		return nil, err
	}
	return []byte(st), nil
}
