package backend

import (
	"fmt"
	stdlog "log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/logutil"
)

// NewReverseProxy creates an http.ReverseProxy which can forward requests to
// one or more endpoints.
//
// The endpoint URLs are passed in through the targets slice. Each endpoint
// must have a corresponding API key in the same position in the keys slice.
//
// The tags will be added as a header to all proxied requests.
// For more details please see multiTransport.
func NewReverseProxy(target *url.URL, apiKey string) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)
	via := fmt.Sprintf("appsec-agent %s", info.Version)
	director := proxy.Director
	proxy.Director = func(req *http.Request) {
		director(req)
		req.Header.Del("X-Api-Version")
		req.Header.Set("Via", via)
		req.Header.Set("Dd-Api-Key", apiKey)
		b, err := httputil.DumpRequest(req, false)
		if err != nil {
			panic(err)
		}
		stdlog.Println(string(b))
		// TODO: add the container tag
		//const headerContainerID = "Datadog-Container-ID"
		//if containerID := req.Header.Get(headerContainerID); containerID != "" {
		//	if ctags := getContainerTags(containerID); ctags != "" {
		//		req.Header.Set("X-Datadog-Container-Tags", ctags)
		//	}
		//}
	}
	proxy.ErrorLog = stdlog.New(logutil.NewThrottled(5, 10*time.Second), "appsec.Proxy: ", 0)
	return proxy
}
