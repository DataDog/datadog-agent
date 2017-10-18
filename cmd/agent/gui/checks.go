package gui

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
	"github.com/gorilla/mux"
	yaml "gopkg.in/yaml.v2"
)

// Adds the specific handlers for /checks/ endpoints
func checkHandler(r *mux.Router) {
	r.HandleFunc("/running", http.HandlerFunc(sendRunningChecks)).Methods("POST")
	r.HandleFunc("/run/{name}", http.HandlerFunc(runCheck)).Methods("POST")
	r.HandleFunc("/run/{name}/once", http.HandlerFunc(runCheckOnce)).Methods("POST")
	r.HandleFunc("/reload/{name}", http.HandlerFunc(reloadCheck)).Methods("POST")
	r.HandleFunc("/getConfig/{fileName}", http.HandlerFunc(getCheckConfigFile)).Methods("POST")
	r.HandleFunc("/setConfig/{fileName}", http.HandlerFunc(setCheckConfigFile)).Methods("POST")
	r.HandleFunc("/list/{fileType}", http.HandlerFunc(listFiles)).Methods("POST")
}

// Sends a list of all the current running checks
func sendRunningChecks(w http.ResponseWriter, r *http.Request) {
	html, e := renderRunningChecks()
	if e != nil {
		w.Write([]byte("Error generating status html: " + e.Error()))
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// Schedules a specific check
func runCheck(w http.ResponseWriter, r *http.Request) {
	// Fetch the desired check
	name := mux.Vars(r)["name"]
	instances := common.AC.GetChecksByName(name)

	for _, ch := range instances {
		common.Coll.RunCheck(ch)
	}
	log.Infof("Scheduled new check: " + name)
	w.Write([]byte("Scheduled new check:" + name))
}

// Runs a specified check once
func runCheckOnce(w http.ResponseWriter, r *http.Request) {
	response := make(map[string]string)
	// Fetch the desired check
	name := mux.Vars(r)["name"]
	instances := common.AC.GetChecksByName(name)
	if len(instances) == 0 {
		html, e := renderError(name)
		if e != nil {
			html = "Error generating html: " + e.Error()
		}

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

		stats = append(stats, s)
	}

	// Render the stats
	html, e := renderCheck(name, stats)
	if e != nil {
		response["success"] = ""
		response["html"] = "Error generating html: " + e.Error()
		res, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		w.Write(res)
		return
	}

	response["success"] = "true"
	response["html"] = html
	res, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	w.Write(res)
}

// Reloads a running check
func reloadCheck(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	instances := common.AC.GetChecksByName(name)
	if len(instances) == 0 {
		log.Errorf("Can't reload " + name + ": check has no new instances.")
		w.Write([]byte("Can't reload " + name + ": check has no new instances"))
		return
	}

	killed, e := common.Coll.ReloadAllCheckInstances(name, instances)
	if e != nil {
		log.Errorf("Error reloading check: " + e.Error())
		w.Write([]byte("Error reloading check: " + e.Error()))
		return
	}

	log.Infof("Removed %v old instance(s) and started %v new instance(s) of %s", len(killed), len(instances), name)
	w.Write([]byte(fmt.Sprintf("Removed %v old instance(s) and started %v new instance(s) of %s", len(killed), len(instances), name)))
}

// Sends the specified config (.yaml) file
func getCheckConfigFile(w http.ResponseWriter, r *http.Request) {
	fileName := mux.Vars(r)["fileName"]
	paths := getAllPaths("yaml")

	var file []byte
	var e error
	for _, path := range paths {
		file, e = ioutil.ReadFile(path + "/" + fileName)
		if e == nil {
			break
		} else if e != nil && !strings.Contains(e.Error(), "no such file") {
			w.Write([]byte("Error reading check file: " + e.Error()))
			return
		}
	}
	if file == nil {
		w.Write([]byte("Error: Couldn't find " + fileName))
		return
	}

	w.Header().Set("Content-Type", "text")
	w.Write(file)
}

type configFormat struct {
	ADIdentifiers []string    `yaml:"ad_identifiers"`
	DockerImages  []string    `yaml:"docker_images"`
	InitConfig    interface{} `yaml:"init_config"`
	Instances     []check.ConfigRawMap
}

// Overwrites a specific check's configuration (yaml) file with new data
// or makes a new config file for that check, if there isn't one yet
func setCheckConfigFile(w http.ResponseWriter, r *http.Request) {
	fileName := mux.Vars(r)["fileName"]

	payload, e := parseBody(r)
	if e != nil {
		w.Write([]byte(e.Error()))
	}
	data := []byte(payload.Config)

	// Check that the data is actually a valid yaml file
	cf := configFormat{}
	e = yaml.Unmarshal(data, &cf)
	if e != nil {
		w.Write([]byte("Error: " + e.Error()))
		return
	}
	if len(cf.Instances) < 1 {
		w.Write([]byte("Configuration file contains no valid instances"))
		return
	}

	// Write new configs to custom checks directory
	path := config.Datadog.GetString("confd_path") + "/" + fileName
	e = ioutil.WriteFile(path, data, 0644)
	if e != nil {
		w.Write([]byte("Error writing to " + fileName + ": " + e.Error()))
		return
	}

	log.Infof("Successfully wrote new " + fileName + " config file.")
	w.Write([]byte("Success"))
}

// Sends a list containing the names of all the specified type of files
func listFiles(w http.ResponseWriter, r *http.Request) {
	// Get the directories where these files might reside
	fileType := mux.Vars(r)["fileType"]
	paths := getAllPaths(fileType)

	// Read all the directories
	filenames := []string{}
	lookup := make(map[string]bool)
	for _, path := range paths {
		fs, e := readDirectory(path)
		if e == nil {
			sort.Strings(fs)
			for _, name := range fs {
				// Only include each file name once (could be in multiple locations),
				// & if a default config is found but a non-default version exists, don't include
				// the default one (Note that the non-default directory is read first)
				trimmed := name
				if i := strings.Index(name, ".default"); i != -1 {
					trimmed = name[:i]
				}
				if _, exists := lookup[trimmed]; !exists {
					filenames = append(filenames, name)
					lookup[trimmed] = true
				}
			}
		}
	}

	if len(filenames) == 0 {
		switch fileType {
		case "py":
			w.Write([]byte("No check (.py) files found."))
		case "yaml":
			w.Write([]byte("No configuration (.yaml) files found."))
		default:
			w.Write([]byte("No " + fileType + " files found."))
		}
		return
	}

	res, _ := json.Marshal(filenames)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(res))
}

// Helper function which returns the appropriate paths for finding checks/check configs
func getAllPaths(fileType string) []string {
	if fileType == "py" {
		return []string{
			filepath.Join(common.GetDistPath(), "checks.d"), // Custom checks
			config.Datadog.GetString("additional_checksd"),  // Custom checks
			common.PyChecksPath,                             // Integrations-core checks
		}
	} else if fileType == "yaml" {
		return []string{
			config.Datadog.GetString("confd_path"),        // Custom checks
			filepath.Join(common.GetDistPath(), "conf.d"), // Default check configs
		}
	}
	return []string{}
}

// Helper function which returns all the filenames in a directory
func readDirectory(path string) ([]string, error) {
	var filenames []string
	dir, e := os.Open(path)
	if e != nil {
		return filenames, e
	}
	defer dir.Close()

	files, e := dir.Readdir(-1)
	if e != nil {
		return filenames, e
	}

	for _, file := range files {
		if file.Mode().IsRegular() {
			filenames = append(filenames, file.Name())
		}
	}

	return filenames, nil
}
