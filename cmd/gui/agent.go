package gui

import (
	"encoding/json"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/version"
	log "github.com/cihub/seelog"
)

func fetch(w http.ResponseWriter, req string) {
	log.Infof("GUI - Received request to fetch " + req)
	switch req {

	case "status":
		// ALTERNATIVE
		//status, e := status.GetAndFormatStatus()

		status, e := status.GetStatus() // returns a map[string]interface{}
		if e != nil {
			log.Errorf("GUI - Error getting status: " + e.Error())
			w.Write([]byte("Error getting status: " + e.Error()))
			return
		}

		res, e := json.Marshal(status)
		if e != nil {
			log.Errorf("GUI - Error marshalling status: " + e.Error())
			w.Write([]byte("Error marshalling status: " + e.Error()))
			return
		}
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

	case "logs":

		// TODO

		w.Write([]byte("Not implemented yet."))

	case "conf":

		// TODO

		w.Write([]byte("Not implemented yet."))

	default:
		w.Write([]byte("Received unknown fetch request: " + req))
		log.Infof("GUI - Received unknown fetch request: %v ", req)
	}
}

func set(w http.ResponseWriter, req string) {
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

	case "conf":

		// TODO

		w.Write([]byte("Not implemented yet."))

	}
}
