package gui

import (
	"encoding/json"
	"expvar"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/version"
	log "github.com/cihub/seelog"
)

func fetch(w http.ResponseWriter, m Message) {
	log.Infof("GUI - Received request to fetch " + m.Data)
	switch m.Data {

	case "status":
		sendStatus(w, m.Payload)

	case "version":
		sendVersion(w)

	case "hostname":
		sendHostname(w)

	case "config_file":
		sendConfig(w)

	case "conf_list":
		sendConfFileList(w)

	case "log":
		sendLog(w, m.Payload, true)

	case "log-no-flip":
		sendLog(w, m.Payload, false)

	case "check_config":
		sendCheckConfig(w, m.Payload)

	case "running_checks":
		sendRunningChecks(w)

	default:
		w.Write([]byte("Received unknown fetch request: " + m.Data))
		log.Infof("GUI - Received unknown fetch request: %v ", m.Data)
	}
}

// Sends the current agent version
func sendVersion(w http.ResponseWriter) {
	version, e := version.New(version.AgentVersion)
	if e != nil {
		log.Errorf("GUI - Error getting version: " + e.Error())
		w.Write([]byte("Error getting version: " + e.Error()))
		return
	}

	res, _ := json.Marshal(version)
	w.Header().Set("Content-Type", "application/json")
	w.Write(res)
}

// Sends the agent's hostname
func sendHostname(w http.ResponseWriter) {
	hostname, e := util.GetHostname()
	if e != nil {
		log.Errorf("GUI - Error getting hostname: " + e.Error())
		w.Write([]byte("Error getting hostname: " + e.Error()))
		return
	}

	res, _ := json.Marshal(hostname)
	w.Header().Set("Content-Type", "application/json")
	w.Write(res)
}

// Sends the configuration (aka datadog.yaml) file
func sendConfig(w http.ResponseWriter) {
	path := config.Datadog.ConfigFileUsed()
	settings, e := ioutil.ReadFile(path)
	if e != nil {
		log.Errorf("GUI - Error reading config file: " + e.Error())
		w.Write([]byte("Error reading config file: " + e.Error()))
		return
	}

	w.Header().Set("Content-Type", "text")
	w.Write(settings)
}

// Sends a list containing the names of all the files in the conf.d directory
func sendConfFileList(w http.ResponseWriter) {
	path := config.Datadog.GetString("confd_path")
	dir, e := os.Open(path)
	if e != nil {
		log.Errorf("GUI - Error opening conf.d directory: " + e.Error())
		w.Write([]byte("Error opening conf.d directory: " + e.Error()))
		return
	}
	defer dir.Close()

	// Read the names of all the files in the configured conf.d directory
	files, e := dir.Readdir(-1)
	if e != nil {
		log.Errorf("GUI - Error reading conf.d directory: " + e.Error())
		w.Write([]byte("Error reading conf.d directory: " + e.Error()))
		return
	}
	var filenames []string
	for _, file := range files {
		if file.Mode().IsRegular() {
			filenames = append(filenames, file.Name())
		}
	}

	// If there are no files, send a relevant message
	if len(filenames) == 0 {
		w.Write([]byte("Empty directory:" + path))
		return
	}

	res, _ := json.Marshal(filenames)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(res))
}

// Sends the specified log file
func sendLog(w http.ResponseWriter, name string, flip bool) {
	// Get the path to agent.log and trim it to make it the path to all logs
	path := config.Datadog.GetString("log_file")
	path = path[0 : len(path)-9]

	logFile, e := ioutil.ReadFile(path + name + ".log")
	if e != nil {
		log.Errorf("GUI - Error reading " + name + " log file: " + e.Error())
		w.Write([]byte("Error reading " + name + " log file: " + e.Error()))
		return
	}

	html := strings.Replace(string(logFile), "\n", "<br>", -1)

	if flip {
		// Reverse the order so that the bottom of the file is read first
		arr := strings.Split(string(logFile), "\n")
		for i, j := 0, len(arr)-1; i < j; i, j = i+1, j-1 {
			arr[i], arr[j] = arr[j], arr[i]
		}
		html = strings.Join(arr, "<br>")
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// Sends the specified yaml file (from the conf.d directory)
func sendCheckConfig(w http.ResponseWriter, name string) {
	path := config.Datadog.GetString("confd_path") + "/" + name

	file, e := ioutil.ReadFile(path)
	if e != nil {
		log.Errorf("GUI - Error reading check file: " + e.Error())
		w.Write([]byte("Error reading check file: " + e.Error()))
		return
	}

	w.Header().Set("Content-Type", "text")
	w.Write(file)
}

// Sends a list of all the current running checks
func sendRunningChecks(w http.ResponseWriter) {
	runnerStatsJSON := []byte(expvar.Get("runner").String())
	runnerStats := make(map[string]interface{})
	json.Unmarshal(runnerStatsJSON, &runnerStats)

	// Parse runnerStatsJSON to get the names of all the current running checks
	var checksList []string
	if checks, ok := runnerStats["Checks"].(map[string]interface{}); ok {
		for _, ch := range checks {
			if check, ok := ch.(map[string]interface{}); ok {
				if name, ok := check["CheckName"].(string); ok {
					checksList = append(checksList, name)
				}
			}
		}
	}

	res, _ := json.Marshal(checksList)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(res))
}
