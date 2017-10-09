package gui

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
	log "github.com/cihub/seelog"
)

func set(w http.ResponseWriter, m Message) {
	switch m.Data {

	case "flare":
		makeFlare(w, m.Payload)

	case "config_file":
		setConfigFile(w, m.Payload)

	case "check_config":
		setCheckConfigFile(w, m.Payload)

	case "run_check_once":
		runCheckOnce(w, m.Payload)

	case "run_check":
		runCheck(w, m.Payload)

	case "reload_check":
		reloadCheck(w, m.Payload)

	case "restart":
		restartAgent(w)

	default:
		w.Write([]byte("Received unknown set request: " + m.Data))
		log.Infof("GUI - Received unknown set request: " + m.Data)

	}
}

// Makes a new flare
func makeFlare(w http.ResponseWriter, payload string) {
	data := strings.Fields(payload)
	if len(data) != 2 {
		w.Write([]byte("Incorrect flare data format: " + payload))
		return
	}
	customerEmail := data[0]
	caseID := data[1]

	// Initiate the flare locally
	filePath, e := flare.CreateArchive(true, common.GetDistPath(), common.PyChecksPath)
	if e != nil {
		w.Write([]byte("Error creating flare zipfile: " + e.Error()))
		log.Errorf("GUI - Error creating flare zipfile: " + e.Error())
		return
	}

	// Send the flare
	res, e := flare.SendFlare(filePath, caseID, customerEmail)
	if e != nil {
		w.Write([]byte("Flare zipfile successfully created: " + filePath + "<br><br>" + e.Error()))
		log.Errorf("GUI - Flare zipfile successfully created: " + filePath + "<br><br>" + e.Error())
		return
	}

	w.Write([]byte("Flare zipfile successfully created: " + filePath + "<br><br>" + res))
	log.Errorf("GUI - Flare zipfile successfully created: " + filePath + "<br><br>" + res)
	return
}

// Overwrites the main config file (datadog.yaml) with new data
func setConfigFile(w http.ResponseWriter, data string) {
	path := config.Datadog.ConfigFileUsed()

	dataB := []byte(data)
	e := ioutil.WriteFile(path, dataB, 0644)
	if e != nil {
		log.Errorf("GUI - Error writing to config file: " + e.Error())
		w.Write([]byte("Error writing to config file: " + e.Error()))
		return
	}

	log.Infof("GUI - Successfully wrote new config file.")
	w.Write([]byte("Success"))
}

// Overwrites a specific check's configuration (yaml) file with new data
// or makes a new config file for that check, if there isn't one yet
func setCheckConfigFile(w http.ResponseWriter, payload string) {
	i := strings.Index(payload, " ")
	name := payload[0:i]
	path := config.Datadog.GetString("confd_path")
	path += "/" + name

	payload = payload[i+1 : len(payload)]
	data := []byte(payload)

	e := ioutil.WriteFile(path, data, 0644)
	if e != nil {
		log.Errorf("GUI - Error writing to " + name + ": " + e.Error())
		w.Write([]byte("Error writing to " + name + ": " + e.Error()))
		return
	}

	log.Infof("GUI - Successfully wrote new " + name + " config file.")
	w.Write([]byte("Success"))
}

// Runs a specified check once
func runCheckOnce(w http.ResponseWriter, name string) {
	// Fetch the desired check
	instances := common.AC.GetChecksByName(name)
	if len(instances) == 0 {
		html, e := renderError(name)
		if e != nil {
			log.Errorf("GUI - Error generating html: " + e.Error())
			w.Write([]byte("Error generating html: " + e.Error()))
			return
		}

		response := make(map[string]string)
		response["success"] = "" // empty string evaluates to false in JS
		response["html"] = html
		res, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		w.Write(res)
		return
	}

	// Run the check intance(s) once, as a test
	stats := []*check.Stats{}
	for _, ch := range instances {
		s := check.NewStats(ch)

		t0 := time.Now()
		err := ch.Run()
		warnings := ch.GetWarnings()
		mStats, _ := ch.GetMetricStats()
		s.Add(time.Since(t0), err, warnings, mStats)

		// Without a small delay some of the metrics will not show up
		time.Sleep(100 * time.Millisecond)

		// TODO(?) get aggregator metrics

		stats = append(stats, s)
	}

	// Render the stats
	html, e := renderCheck(name, stats)
	if e != nil {
		log.Errorf("GUI - Error generating html: " + e.Error())
		w.Write([]byte("Error generating html: " + e.Error()))
		return
	}

	response := make(map[string]string)
	response["success"] = "true"
	response["html"] = html
	res, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	w.Write(res)
}

// Schedules a specific check
func runCheck(w http.ResponseWriter, name string) {
	// Fetch the desired check
	instances := common.AC.GetChecksByName(name)

	for _, ch := range instances {
		common.Coll.RunCheck(ch)
	}
	w.Write([]byte("Ran check " + name))
}

// Reloads a running check
func reloadCheck(w http.ResponseWriter, name string) {
	instances := common.AC.GetChecksByName(name)
	if len(instances) == 0 {
		log.Errorf("GUI - Can't reload " + name + ": check has no new instances.")
		w.Write([]byte("Can't reload " + name + ": check has no new instances"))
		return
	}

	killed, e := common.Coll.ReloadAllCheckInstances(name, instances)
	if e != nil {
		log.Errorf("GUI - Error reloading check: " + e.Error())
		w.Write([]byte("Error reloading check: " + e.Error()))
		return
	}

	w.Write([]byte(fmt.Sprintf("Removed %v old instance(s) and started %v new instance(s) of %s", len(killed), len(instances), name)))
}

// Tells service manager to restart the agent
func restartAgent(w http.ResponseWriter) {
	e := common.Restart()
	if e != nil {
		w.Write([]byte(e.Error()))
		return
	}
	w.Write([]byte("Success"))
}
