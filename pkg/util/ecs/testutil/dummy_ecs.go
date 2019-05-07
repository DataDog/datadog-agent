package testutil

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
)

// DummyECS allows tests to mock a ECS's responses
type DummyECS struct {
	Requests     chan *http.Request
	TaskListJSON string
	MetadataJSON string
}

// NewDummyECS create a mock of the ECS api
func NewDummyECS() (*DummyECS, error) {
	return &DummyECS{Requests: make(chan *http.Request, 3)}, nil
}

// ServeHTTP handles the http requests
func (d *DummyECS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("dummyECS received %s on %s", r.Method, r.URL.Path)
	d.Requests <- r
	switch r.URL.Path {
	case "/":
		w.Write([]byte(`{"AvailableCommands":["/v2/metadata","/v1/tasks","/license"]}`))
	case "/v1/tasks":
		w.Write([]byte(d.TaskListJSON))
	case "/v2/metadata":
		w.Write([]byte(d.MetadataJSON))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// Start starts the HTTP server
func (d *DummyECS) Start() (*httptest.Server, int, error) {
	ts := httptest.NewServer(d)
	ecsAgentURL, err := url.Parse(ts.URL)
	if err != nil {
		return nil, 0, err
	}
	ecsAgentPort, err := strconv.Atoi(ecsAgentURL.Port())
	if err != nil {
		return nil, 0, err
	}
	return ts, ecsAgentPort, nil
}
