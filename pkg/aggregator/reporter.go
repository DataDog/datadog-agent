package aggregator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
)

// Report metrics to the API
func Report(series []*Serie, apiKey string) {
	if len(series) == 0 {
		log.Info("No series to flush")
		return
	}

	url := fmt.Sprintf("%s/api/v1/series?api_key=%s", config.Datadog.GetString("dd_url"), apiKey)
	log.Infof("Flushing %d series to %s", len(series), url)

	// Encode payload to JSON
	data := map[string][]*Serie{
		"series": series,
	}
	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(data)
	if err != nil {
		log.Errorf("Error serializing payload: %v", err)
		return
	}

	// Prepare request
	req, err := http.NewRequest("POST", url, reqBody)
	if err != nil {
		log.Errorf("Error initializing request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Errorf("Error sending payload: %v", err)
		return
	}
	defer resp.Body.Close()

	// 4xx or 5xx status code
	if resp.StatusCode >= 400 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Errorf("Can't read response body: %s", err)
		}
		log.Errorf("Unexpected response status code %d. Response body: %s", resp.StatusCode, body)
		return
	}

	log.Infof("Flush successful")
}
