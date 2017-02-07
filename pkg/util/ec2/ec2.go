package ec2

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

// declare these as vars not const to ease testing
var (
	metadataURL     = "http://169.254.169.254/latest/meta-data"
	timeout         = 100 * time.Millisecond
	defaultPrefixes = []string{"ip-", "domu"}
)

// GetInstanceID returns the EC2 instance id for this host
func GetInstanceID() (string, error) {
	res, err := getResponse(metadataURL + "/instance-id")

	defer res.Body.Close()
	all, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("unable to retrieve Instance ID, %s", err)
	}

	return string(all), nil
}

func getResponse(url string) (*http.Response, error) {
	client := http.Client{
		Timeout: timeout,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("status code %d trying to fetch %s", res.StatusCode, url)
	}

	return res, nil
}

// IsDefaultHostname returns whether the given hostname is a default one for EC2
func IsDefaultHostname(hostname string) bool {
	hostname = strings.ToLower(hostname)
	isDefault := false
	for _, val := range defaultPrefixes {
		isDefault = isDefault || strings.HasPrefix(hostname, val)
	}
	return isDefault
}
