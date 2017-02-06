package gce

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

const (
	metadataURL               = "http://169.254.169.254/computeMetadata/v1/"
	timeout     time.Duration = 300 * time.Millisecond
)

// GetHostname returns the hostname querying GCE Metadata api
func GetHostname() (string, error) {
	client := http.Client{
		Timeout: timeout,
	}

	url := metadataURL + "instance/hostname"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Add("Metadata-Flavor", "Google")
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return "", fmt.Errorf("GCE hostname, status code %d trying to GET %s", res.StatusCode, url)
	}

	all, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("GCE hostname, error reading response body: %s", err)
	}

	return string(all), nil
}
