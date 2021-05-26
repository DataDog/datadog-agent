// +build linux_bpf

package http

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	nethttp "net/http"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/netlink/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/stretchr/testify/require"
)

func TestHTTPMonitorIntegration(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < kernel.VersionCode(4, 1, 0) {
		t.Skip("HTTP feature not available on pre 4.1.0 kernels")
	}

	targetAddr := "localhost:8080"
	serverAddr := "localhost:8080"
	testHTTPMonitor(t, targetAddr, serverAddr, 100)
}

func TestHTTPMonitorIntegrationWithNAT(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < kernel.VersionCode(4, 1, 0) {
		t.Skip("HTTP feature not available on pre 4.1.0 kernels")
	}

	// SetupDNAT sets up a NAT translation from 2.2.2.2 to 1.1.1.1
	testutil.SetupDNAT(t)
	defer testutil.TeardownDNAT(t)

	targetAddr := "2.2.2.2:8080"
	serverAddr := "1.1.1.1:8080"
	testHTTPMonitor(t, targetAddr, serverAddr, 10)
}

func testHTTPMonitor(t *testing.T, targetAddr, serverAddr string, numReqs int) {
	srvDoneFn := serverSetup(t, serverAddr)
	defer srvDoneFn()

	monitor, err := NewMonitor(config.New())
	require.NoError(t, err)
	err = monitor.Start()
	require.NoError(t, err)
	defer monitor.Stop()

	// Perform a number of random requests
	requestFn := requestGenerator(t, targetAddr)
	var requests []*nethttp.Request
	for i := 0; i < numReqs; i++ {
		requests = append(requests, requestFn())
	}

	// Ensure all captured transactions get sent to user-space
	time.Sleep(10 * time.Millisecond)
	stats := monitor.GetHTTPStats()

	// Assert all requests made were correctly captured by the monitor
	for _, req := range requests {
		includesRequest(t, stats, req)
	}
}

func includesRequest(t *testing.T, allStats map[Key]RequestStats, req *nethttp.Request) {
	expectedStatus := statusFromPath(req.URL.Path)
	for key, stats := range allStats {
		i := expectedStatus/100 - 1
		if key.Path == req.URL.Path && stats[i].Count == 1 {
			return
		}
	}

	t.Errorf(
		"could not find HTTP transaction matching the following criteria:\n path=%s method=%s status=%d",
		req.URL.Path,
		req.Method,
		expectedStatus,
	)
}

// serverSetup spins up a HTTP test server that returns the status code included in the URL
// Example:
// * GET /200/foo returns a 200 status code;
// * PUT /404/bar returns a 404 status code;
func serverSetup(t *testing.T, addr string) func() {
	handler := func(w nethttp.ResponseWriter, req *nethttp.Request) {
		statusCode := statusFromPath(req.URL.Path)
		io.Copy(ioutil.Discard, req.Body)
		w.WriteHeader(statusCode)
	}

	srv := &nethttp.Server{
		Addr:         addr,
		Handler:      nethttp.HandlerFunc(handler),
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
	}

	srv.SetKeepAlivesEnabled(false)

	go func() {
		_ = srv.ListenAndServe()
	}()

	return func() { srv.Shutdown(context.Background()) }
}

func requestGenerator(t *testing.T, targetAddr string) func() *nethttp.Request {
	var (
		methods     = []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
		statusCodes = []int{200, 300, 400, 500}
		random      = rand.New(rand.NewSource(time.Now().Unix()))
		idx         = 0
		client      = new(nethttp.Client)
	)

	return func() *nethttp.Request {
		idx++
		method := methods[random.Intn(len(methods))]
		status := statusCodes[random.Intn(len(statusCodes))]
		url := fmt.Sprintf("http://%s/%d/request-%d", targetAddr, status, idx)
		req, err := nethttp.NewRequest(method, url, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		return req
	}
}

var pathParser = regexp.MustCompile(`/(\d{3})/.+`)

func statusFromPath(path string) (status int) {
	matches := pathParser.FindStringSubmatch(path)
	if len(matches) == 2 {
		status, _ = strconv.Atoi(matches[1])
	}

	return
}
