package gui

import (
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
	log "github.com/cihub/seelog"
)

func set(w http.ResponseWriter, req string, payload string) {
	switch req {

	case "flare":
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

		w.Write([]byte("User " + customerEmail + " created flare[" + caseID + "]. Zipfile: " + filePath))
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
	case "settings":
		path := config.Datadog.ConfigFileUsed()

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
