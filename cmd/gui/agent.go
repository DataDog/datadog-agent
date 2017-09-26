package gui

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/version"
	log "github.com/cihub/seelog"
)

func fetch(w http.ResponseWriter, req string) {
	log.Infof("GUI - Received request to fetch " + req)
	switch req {

	case "status":
		status, e := status.GetStatus()
		if e != nil {
			log.Errorf("GUI - Error getting status: " + e.Error())
			w.Write([]byte("Error getting status: " + e.Error()))
			return
		}

		res, _ := json.Marshal(status)
		w.Header().Set("Content-Type", "application/json")
		w.Write(res)

	case "version":
		version, e := version.New(version.AgentVersion)
		if e != nil {
			log.Errorf("GUI - Error getting version: " + e.Error())
			w.Write([]byte("Error getting version: " + e.Error()))
			return
		}

		res, _ := json.Marshal(version)
		w.Header().Set("Content-Type", "application/json")
		w.Write(res)

	case "hostname":
		hostname, e := util.GetHostname()
		if e != nil {
			log.Errorf("GUI - Error getting hostname: " + e.Error())
			w.Write([]byte("Error getting hostname: " + e.Error()))
			return
		}

		res, _ := json.Marshal(hostname)
		w.Header().Set("Content-Type", "application/json")
		w.Write(res)

	case "settings":
		path := config.Datadog.ConfigFileUsed()
		settings, e := ioutil.ReadFile(path)
		if e != nil {
			log.Errorf("GUI - Error reading config file: " + e.Error())
			w.Write([]byte("Error reading config file: " + e.Error()))
			return
		}

		w.Header().Set("Content-Type", "text")
		w.Write(settings)

	case "conf_list":

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

		res, _ := json.Marshal(filenames)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(res))

	default:
		w.Write([]byte("Received unknown fetch request: " + req))
		log.Infof("GUI - Received unknown fetch request: %v ", req)
	}
}

func set(w http.ResponseWriter, req string, payload string) {
	switch req {

	case "flare":
		/*
			filePath, err := flare.CreateArchive(false, common.GetDistPath(), common.PyChecksPath)
			if err != nil || filePath == "" {
				if err != nil {
					log.Errorf("The flare failed to be created: %s", err)
				} else {
					log.Warnf("The flare failed to be created")
				}
				http.Error(w, err.Error(), 500)
			}
			w.Write([]byte(filePath))
		*/

		w.Write([]byte("Not implemented yet."))

	case "settings":
		path := config.Datadog.ConfigFileUsed()

		/*
			fileInfo, _ := os.Stat(path)
			mode := fileInfo.Mode()

			log.Infof("GUI - %v file mode: %v", path, mode)
			user, _ := user.Current()
			log.Infof("GUI - Writing to %v with 0644 permissions from user %v", path, user)

			// sudo -u root ./bin/agent/agent start
			// stat -c '%U' /etc/dd-agent/datadog.yaml
		*/

		data := []byte(payload)
		e := ioutil.WriteFile(path, data, 0644)
		if e != nil {
			log.Errorf("GUI - Error writing to config file: " + e.Error())
			w.Write([]byte("Error writing to config file: " + e.Error()))
			return
		}

		log.Infof("GUI - Successfully wrote new config file.")
		w.Write([]byte("Success"))

	default:
		w.Write([]byte("Received unknown fetch request: " + req))
		log.Infof("GUI - Received unknown fetch request: %v ", req)

	}
}

func check(w http.ResponseWriter, req string, payload string) {
	switch req {

	case "get_yaml":
		path := config.Datadog.GetString("confd_path")
		path += "/" + payload

		file, e := ioutil.ReadFile(path)
		if e != nil {
			log.Errorf("GUI - Error reading check file: " + e.Error())
			w.Write([]byte("Error reading check file: " + e.Error()))
			return
		}

		w.Header().Set("Content-Type", "text")
		w.Write(file)

	case "set_yaml":
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

		log.Errorf("GUI - Succesfully wrote to " + name + ".")
		w.Write([]byte("Success"))

	default:
		w.Write([]byte("Received unknown fetch request: " + req))
		log.Infof("GUI - Received unknown fetch request: %v ", req)

	}
}
