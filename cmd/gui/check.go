package gui

import (
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
)

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
