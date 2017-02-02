package gce

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	metadataURL               = "http://169.254.169.254/computeMetadata/v1/"
	timeout     time.Duration = 300 * time.Millisecond
)

type metadata struct {
	instance struct {
		hostname string
	}
}

// GetHostname returns the hostname querying GCE Metadata api
func GetHostname() (string, error) {
	md, err := getMetadata()
	if err != nil {
		return "", fmt.Errorf("error fetching GCE hostname, %s", err)
	}
	return md.instance.hostname, nil
}

func getMetadata() (*metadata, error) {
	client := http.Client{
		Timeout: timeout,
	}

	url := metadataURL + "?recursive=true"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Metadata-Flavor", "Google")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("status code %d trying to fetch %s", res.StatusCode, url)
	}

	decoder := json.NewDecoder(res.Body)
	var md metadata
	err = decoder.Decode(&md)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal metadata, %s", err)
	}

	return &md, nil
}
