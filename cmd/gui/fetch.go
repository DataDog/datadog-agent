package gui

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/version"
	log "github.com/cihub/seelog"
)

func fetch(w http.ResponseWriter, req string) {
	log.Infof("GUI - Received request to fetch " + req)
	switch req {

	case "generalStatus", "collectorStatus":
		sendStatus(w, req)

	case "version":
		sendVersion(w)

	case "hostname":
		sendHostname(w)

	case "settings":
		sendConfig(w)

	case "conf_list":
		sendConfFileList(w)

	case "agentLog", "collectorLog", "dogstatsdLog", "forwarderLog", "jmxfetchLog":
		sendLog(w, req[0:len(req)-3], true)

	case "agentLog-noflip", "collectorLog-noflip", "dogstatsdLog-noflip", "forwarderLog-noflip", "jmxfetchLog-noflip":
		sendLog(w, req[0:len(req)-10], false)

	default:
		w.Write([]byte("Received unknown fetch request: " + req))
		log.Infof("GUI - Received unknown fetch request: %v ", req)
	}
}

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

func sendConfFileList(w http.ResponseWriter) {
	path := config.Datadog.GetString("confd_path")
	dir, e := os.Open(path)
	if e != nil {
		log.Errorf("GUI - Error opening conf.d directory: " + e.Error())
		w.Write([]byte("Error opening conf.d directory: " + e.Error()))
		return
	}
	defer dir.Close()

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

	// TODO (?) also read the files in ./bin/agent/dist/conf.d/

	res, _ := json.Marshal(filenames)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(res))
}

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
