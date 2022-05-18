package metadata

import (
	"io/ioutil"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultUrl = "http://metadata.google.internal/computeMetadata/v1/instance/id"
const defaultTimeout = 300 * time.Millisecond

type MetadataConfig struct {
	url     string
	timeout time.Duration
}

func GetDefaultConfig() *MetadataConfig {
	return &MetadataConfig{
		url:     defaultUrl,
		timeout: defaultTimeout,
	}
}

func GetContainerId(config *MetadataConfig) string {
	client := &http.Client{
		Timeout: config.timeout,
	}
	req, err := http.NewRequest(http.MethodGet, config.url, nil)
	if err != nil {
		log.Error("unable to build the metadata request, defaulting to unknown-id")
		return "unknown-id"
	}
	req.Header.Add("Metadata-Flavor", "Google")
	res, err := client.Do(req)
	if err != nil {
		log.Error("unable to get the instance id, defaulting to unknown-id")
		return "unknown-id"
	} else {
		data, _ := ioutil.ReadAll(res.Body)
		return string(data)
	}
}
