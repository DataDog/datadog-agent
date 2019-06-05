package writer

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// uid is an atomically incremented ID, used by the expectResponses function to
// create payload IDs for the test server.
var uid uint64

// expectResponses creates a new payload for the test server. The test server will
// respond with the given status codes, in the given order, for each subsequent
// request, in rotation.
func expectResponses(codes ...int) *payload {
	if len(codes) == 0 {
		codes = []int{http.StatusOK}
	}
	var str bytes.Buffer
	str.WriteString(strconv.FormatUint(atomic.AddUint64(&uid, 1), 10))
	str.WriteString("|")
	for i, code := range codes {
		if i > 0 {
			str.WriteString(",")
		}
		str.WriteString(strconv.Itoa(code))
	}
	return newPayload(str.Bytes(), nil)
}

// newTestServer returns a new, started HTTP test server. Its URL is available
// as a field. To control its responses, send it payloads created by expectResponses.
// By default, the testServer always returns http.StatusOK.
func newTestServer() *testServer {
	srv := &testServer{
		seen: make(map[string]*requestStatus),
	}
	srv.server = httptest.NewServer(srv)
	srv.URL = srv.server.URL
	return srv
}

// testServer is an http.Handler and http.Server which records the number of total,
// failed, retriable and accepted requests. It also allows manipulating it's HTTTP
// status code response by means of the request's body (see expectResponses).
type testServer struct {
	t      *testing.T
	URL    string
	server *httptest.Server

	mu   sync.Mutex
	seen map[string]*requestStatus

	total, retried, failed, accepted uint64
}

// requestStatus keeps track of how many times a custom payload was seen and what
// the next HTTP status code response should be.
type requestStatus struct {
	count int
	codes []int
}

// nextResponse returns the next HTTP response code and advances the count.
func (rs *requestStatus) nextResponse() int {
	statusCode := rs.codes[rs.count%len(rs.codes)]
	rs.count++
	return statusCode
}

func (ts *testServer) Failed() int   { return int(atomic.LoadUint64(&ts.failed)) }
func (ts *testServer) Retried() int  { return int(atomic.LoadUint64(&ts.retried)) }
func (ts *testServer) Total() int    { return int(atomic.LoadUint64(&ts.total)) }
func (ts *testServer) Accepted() int { return int(atomic.LoadUint64(&ts.accepted)) }

// ServeHTTP responds based on the request body.
func (ts *testServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	atomic.AddUint64(&ts.total, 1)
	slurp, err := ioutil.ReadAll(req.Body)
	if err != nil {
		panic(fmt.Sprintf("error reading request body: %v", err))
	}
	defer req.Body.Close()
	statusCode := ts.getNextCode(slurp)
	w.WriteHeader(statusCode)
	switch statusCode / 100 {
	case 5: // 5xx
		atomic.AddUint64(&ts.retried, 1)
	case 2: // 2xx
		atomic.AddUint64(&ts.accepted, 1)
	default:
		atomic.AddUint64(&ts.failed, 1)
	}
}

func (ts *testServer) getNextCode(reqBody []byte) int {
	parts := strings.Split(string(reqBody), "|")
	if len(parts) != 2 {
		// not a special body
		return http.StatusOK
	}
	id := parts[0]
	ts.mu.Lock()
	defer ts.mu.Unlock()
	p, ok := ts.seen[id]
	if !ok {
		parts := strings.Split(parts[1], ",")
		codes := make([]int, len(parts))
		for i, part := range parts {
			code, err := strconv.Atoi(part)
			if err != nil {
				log.Println("testServer: warning: possibly malformed request body")
				return http.StatusOK
			}
			if http.StatusText(code) == "" {
				panic(fmt.Sprintf("testServer: invalid status code: %d", code))
			}
			codes[i] = code
		}
		ts.seen[id] = &requestStatus{codes: codes}
		p = ts.seen[id]
	}
	return p.nextResponse()
}

// Close closes the underlying http.Server.
func (ts *testServer) Close() { ts.server.Close() }
