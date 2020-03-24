package status

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	processnet "github.com/DataDog/datadog-agent/pkg/process/net"
)

func getSystemProbeStatus() (map[string]interface{}) {

	processnet.GetRemoteSystemProbeUtil()
	systemProbeDetails := make(map[string]interface{})

	httpClient := http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:    2,
			IdleConnTimeout: 30 * time.Second,
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", config.Datadog.GetString("system_probe_config.sysprobe_socket") )
			},
			TLSHandshakeTimeout:   1 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			ExpectContinueTimeout: 50 * time.Millisecond,
		},
	}

	resp, err := httpClient.Get("http://unix/debug/stats")

	return systemProbeDetails
}


