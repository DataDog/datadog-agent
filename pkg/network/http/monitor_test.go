// +build linux_bpf

package http

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	nethttp "net/http"
	"regexp"
	"strconv"
	"testing"
	"time"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestHTTPMonitorIntegration(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < kernel.VersionCode(4, 1, 0) {
		t.Skip("HTTP feature not available on pre 4.1.0 kernels")
	}

	srvDoneFn := serverSetup(t)
	defer srvDoneFn()

	// Create a monitor that simply buffers all HTTP requests
	var buffer []httpTX
	handlerFn := func(transactions []httpTX) {
		buffer = append(buffer, transactions...)
	}
	monitor, doneFn := monitorSetup(t, handlerFn)
	defer doneFn()

	// Perform a number of random requests
	requestFn := requestGenerator(t)
	var requests []*nethttp.Request
	for i := 0; i < 50; i++ {
		requests = append(requests, requestFn())
	}

	// Ensure all captured transactions get sent to user-space
	time.Sleep(10 * time.Millisecond)
	monitor.Sync()

	// Assert all requests made were correctly captured by the monitor
	for _, req := range requests {
		hasMatchingTX(t, req, buffer)
	}
}

func hasMatchingTX(t *testing.T, req *nethttp.Request, transactions []httpTX) {
	expectedStatus := statusFromPath(req.URL.Path)
	for _, tx := range transactions {
		if tx.Path() == req.URL.Path && int(tx.response_status_code) == expectedStatus && tx.Method() == req.Method {
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
func serverSetup(t *testing.T) func() {
	handler := func(w nethttp.ResponseWriter, req *nethttp.Request) {
		statusCode := statusFromPath(req.URL.Path)
		io.Copy(ioutil.Discard, req.Body)
		w.WriteHeader(statusCode)
	}

	srv := &nethttp.Server{
		Addr:         "localhost:8080",
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

func monitorSetup(t *testing.T, handlerFn func([]httpTX)) (*Monitor, func()) {
	mgr, perfHandler := eBPFSetup(t)
	monitor, err := NewMonitor("/proc", mgr, perfHandler)
	require.NoError(t, err)
	monitor.handler = handlerFn

	// Start HTTP monitor
	err = monitor.Start()
	require.NoError(t, err)

	// Start manager
	err = mgr.Start()
	require.NoError(t, err)

	doneFn := func() {
		monitor.Stop()
		mgr.Stop(manager.CleanAll)
	}
	return monitor, doneFn
}

func eBPFSetup(t *testing.T) (*manager.Manager, *ddebpf.PerfHandler) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	pre410Kernel := currKernelVersion < kernel.VersionCode(4, 1, 0)
	if pre410Kernel {
		t.Skip("HTTP feature not available on pre 4.1.0 kernels")
		return nil, nil
	}

	httpPerfHandler := ddebpf.NewPerfHandler(10)
	mgr := netebpf.NewManager(ddebpf.NewPerfHandler(1), httpPerfHandler, false)
	mgrOptions := manager.Options{
		MapSpecEditors: map[string]manager.MapSpecEditor{
			string(probes.HttpInFlightMap): {Type: ebpf.Hash, MaxEntries: 1024, EditorFlag: manager.EditMaxEntries},

			// These maps are unrelated to HTTP but need to have their `MaxEntries` set because the eBPF library loads all of them
			string(probes.ConnMap):            {Type: ebpf.Hash, MaxEntries: 1024, EditorFlag: manager.EditMaxEntries},
			string(probes.TcpStatsMap):        {Type: ebpf.Hash, MaxEntries: 1024, EditorFlag: manager.EditMaxEntries},
			string(probes.PortBindingsMap):    {Type: ebpf.Hash, MaxEntries: 1024, EditorFlag: manager.EditMaxEntries},
			string(probes.UdpPortBindingsMap): {Type: ebpf.Hash, MaxEntries: 1024, EditorFlag: manager.EditMaxEntries},
		},
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
		ActivatedProbes: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					Section: string(probes.SocketHTTPFilter),
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					Section: string(probes.TCPSendMsgReturn),
				},
			},
		},
	}

	for _, p := range mgr.Probes {
		if p.Section != string(probes.SocketHTTPFilter) && p.Section != string(probes.TCPSendMsgReturn) {
			mgrOptions.ExcludedSections = append(mgrOptions.ExcludedSections, p.Section)
		}
	}

	elf, err := netebpf.ReadBPFModule("build", true)
	require.NoError(t, err)
	err = mgr.InitWithOptions(elf, mgrOptions)
	require.NoError(t, err)
	return mgr, httpPerfHandler
}

func requestGenerator(t *testing.T) func() *nethttp.Request {
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
		url := fmt.Sprintf("http://localhost:8080/%d/request-%d", status, idx)
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
