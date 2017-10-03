package gui

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/collector/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/collector/providers"
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
		w.Write([]byte("Error creating the flare zipfile: " + e.Error()))
		log.Errorf("GUI - Error creating the flare zipfile: " + e.Error())
		return
	}

	w.Write([]byte("User " + customerEmail + " created flare[" + caseID + "].<br> Zipfile: " + filePath))
	return

	/* 	While testing, don't actually send a flare
	// Send the flare
	res, e := flare.SendFlare(filePath, caseID, customerEmail)
	if e != nil {
		w.Write([]byte("Error sending the flare: " + e.Error()))
		log.Errorf("GUI - Error sending the flare: " + e.Error())
		return
	}

	log.Infof("GUI - Uploaded a flare to DataDog. Response: " + res)
	w.Write([]byte("Uploaded a flare to DataDog. Response: " + res))
	*/
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
	// Set up a new AutoConfig instance (otherwise, it won't load "old" checks)
	log.Infof("GUI - Creating new AutoConfig instance.")
	ac := autodiscovery.NewAutoConfig(common.Coll)
	for _, loader := range loaders.LoaderCatalog() {
		ac.AddLoader(loader)
	}
	confSearchPaths := []string{config.Datadog.GetString("confd_path")}
	ac.AddProvider(providers.NewFileConfigProvider(confSearchPaths), false)

	// Load the desired check
	cs := ac.GetChecksByName(name)
	if len(cs) == 0 {
		log.Errorf("GUI - Check " + name + " couldn't be loaded.")
		w.Write([]byte("Check " + name + " had a loading error. See Collector Status for more details."))
		return
	}

	// Run the check intance(s)
	stats := []*check.Stats{}
	for _, c := range cs {
		s := check.NewStats(c)

		t0 := time.Now()
		err := c.Run()
		warnings := c.GetWarnings()
		mStats, _ := c.GetMetricStats()
		s.Add(time.Since(t0), err, warnings, mStats)

		// Without a small delay some of the metrics will not show up
		time.Sleep(100 * time.Millisecond)

		// TODO: get aggregator metrics (see below)

		stats = append(stats, s)
	}
	log.Infof("GUI - Ran check "+name+". Stats: %v", stats)

	res, _ := json.Marshal(stats)
	w.Header().Set("Content-Type", "application/json")
	w.Write(res)
}

/*
func getMetrics(agg *aggregator.BufferedAggregator) {
	series := agg.GetSeries()
	if len(series) != 0 {
		fmt.Println("Series: ")
		j, _ := json.MarshalIndent(series, "", "  ")
		fmt.Println(string(j))
	}

	sketches := agg.GetSketches()
	if len(sketches) != 0 {
		fmt.Println("Sketches: ")
		j, _ := json.MarshalIndent(sketches, "", "  ")
		fmt.Println(string(j))
	}

	serviceChecks := agg.GetServiceChecks()
	if len(serviceChecks) != 0 {
		fmt.Println("Service Checks: ")
		j, _ := json.MarshalIndent(serviceChecks, "", "  ")
		fmt.Println(string(j))
	}

	events := agg.GetEvents()
	if len(events) != 0 {
		fmt.Println("Events: ")
		j, _ := json.MarshalIndent(events, "", "  ")
		fmt.Println(string(j))
	}
}
*/
