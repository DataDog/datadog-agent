package gui

import (
	json "github.com/json-iterator/go"
	"html"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/gorilla/mux"
	yaml "gopkg.in/yaml.v2"
)

// Adds the specific handlers for /agent/ endpoints
func agentHandler(r *mux.Router) {
	r.HandleFunc("/ping", http.HandlerFunc(ping)).Methods("POST")
	r.HandleFunc("/status/{type}", http.HandlerFunc(getStatus)).Methods("POST")
	r.HandleFunc("/version", http.HandlerFunc(getVersion)).Methods("POST")
	r.HandleFunc("/hostname", http.HandlerFunc(getHostname)).Methods("POST")
	r.HandleFunc("/log/{flip}", http.HandlerFunc(getLog)).Methods("POST")
	r.HandleFunc("/flare", http.HandlerFunc(makeFlare)).Methods("POST")
	r.HandleFunc("/restart", http.HandlerFunc(restartAgent)).Methods("POST")
	r.HandleFunc("/getConfig", http.HandlerFunc(getConfigFile)).Methods("POST")
	r.HandleFunc("/setConfig", http.HandlerFunc(setConfigFile)).Methods("POST")
}

// Sends a simple reply (for checking connection to server)
func ping(w http.ResponseWriter, r *http.Request) {
	elapsed := time.Now().Unix() - startTimestamp
	w.Write([]byte(strconv.FormatInt(elapsed, 10)))
}

// Sends the current agent status
func getStatus(w http.ResponseWriter, r *http.Request) {
	statusType := mux.Vars(r)["type"]

	status, e := status.GetStatus()
	if e != nil {
		log.Errorf("Error getting status: " + e.Error())
		w.Write([]byte("Error getting status: " + e.Error()))
		return
	}
	json, _ := json.Marshal(status)
	html, e := renderStatus(json, statusType)
	if e != nil {
		w.Write([]byte("Error generating status html: " + e.Error()))
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// Sends the current agent version
func getVersion(w http.ResponseWriter, r *http.Request) {
	version, e := version.New(version.AgentVersion, version.Commit)
	if e != nil {
		log.Errorf("Error getting version: " + e.Error())
		w.Write([]byte("Error: " + e.Error()))
		return
	}

	res, _ := json.Marshal(version)
	w.Header().Set("Content-Type", "application/json")
	w.Write(res)
}

// Sends the agent's hostname
func getHostname(w http.ResponseWriter, r *http.Request) {
	hostname, e := util.GetHostname()
	if e != nil {
		log.Errorf("Error getting hostname: " + e.Error())
		w.Write([]byte("Error: " + e.Error()))
		return
	}

	res, _ := json.Marshal(hostname)
	w.Header().Set("Content-Type", "application/json")
	w.Write(res)
}

// Sends the log file (agent.log)
func getLog(w http.ResponseWriter, r *http.Request) {
	flip, _ := strconv.ParseBool(mux.Vars(r)["flip"])

	logFile := config.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = common.DefaultLogFile
	}

	logFileContents, e := ioutil.ReadFile(logFile)
	if e != nil {
		w.Write([]byte("Error: " + e.Error()))
		return
	}
	escapedLogFileContents := html.EscapeString(string(logFileContents))

	html := strings.Replace(escapedLogFileContents, "\n", "<br>", -1)

	if flip {
		// Reverse the order so that the bottom of the file is read first
		arr := strings.Split(escapedLogFileContents, "\n")
		for i, j := 0, len(arr)-1; i < j; i, j = i+1, j-1 {
			arr[i], arr[j] = arr[j], arr[i]
		}
		html = strings.Join(arr, "<br>")
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// Makes a new flare
func makeFlare(w http.ResponseWriter, r *http.Request) {
	payload, e := parseBody(r)
	if e != nil {
		w.Write([]byte(e.Error()))
	} else if payload.Email == "" || payload.CaseID == "" {
		w.Write([]byte("Error creating flare: missing information"))
		return
	}

	logFile := config.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = common.DefaultLogFile
	}

	filePath, e := flare.CreateArchive(false, common.GetDistPath(), common.PyChecksPath, logFile)
	if e != nil {
		w.Write([]byte("Error creating flare zipfile: " + e.Error()))
		log.Errorf("Error creating flare zipfile: " + e.Error())
		return
	}

	// Send the flare
	res, e := flare.SendFlare(filePath, payload.CaseID, payload.Email)
	if e != nil {
		w.Write([]byte("Flare zipfile successfully created: " + filePath + "<br><br>" + e.Error()))
		log.Errorf("Flare zipfile successfully created: " + filePath + "\n" + e.Error())
		return
	}

	w.Write([]byte("Flare zipfile successfully created: " + filePath + "<br><br>" + res))
	log.Errorf("Flare zipfile successfully created: " + filePath + "\n" + res)
	return
}

// Restarts the agent using the appropriate (platform-specific) restart function
func restartAgent(w http.ResponseWriter, r *http.Request) {
	e := restart()
	if e != nil {
		w.Write([]byte(e.Error()))
		return
	}
	w.Write([]byte("Success"))
}

// Sends the configuration (aka datadog.yaml) file
func getConfigFile(w http.ResponseWriter, r *http.Request) {
	path := config.Datadog.ConfigFileUsed()
	settings, e := ioutil.ReadFile(path)
	if e != nil {
		w.Write([]byte("Error: " + e.Error()))
		return
	}

	w.Header().Set("Content-Type", "text")
	w.Write(settings)
}

// Overwrites the main config file (datadog.yaml) with new data
func setConfigFile(w http.ResponseWriter, r *http.Request) {
	payload, e := parseBody(r)
	if e != nil {
		w.Write([]byte(e.Error()))
	}
	data := []byte(payload.Config)

	// Check that the data is actually a valid yaml file
	cf := make(map[string]interface{})
	e = yaml.Unmarshal(data, &cf)
	if e != nil {
		w.Write([]byte("Error: " + e.Error()))
		return
	}

	path := config.Datadog.ConfigFileUsed()
	e = ioutil.WriteFile(path, data, 0644)
	if e != nil {
		w.Write([]byte("Error: " + e.Error()))
		return
	}

	log.Infof("Successfully wrote new config file.")
	w.Write([]byte("Success"))
}
