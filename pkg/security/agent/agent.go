package agent

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"google.golang.org/grpc"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RuntimeSecurityAgent - main wrapper for the Runtime Security product
type RuntimeSecurityAgent struct {
	logClient *DDClient
	conn      *grpc.ClientConn
	running   atomic.Value
	wg        *sync.WaitGroup
}

// NewRuntimeSecurityAgent - Instantiates a new RuntimeSecurityAgent
func NewRuntimeSecurityAgent(systemProbeAddr string) (*RuntimeSecurityAgent, error) {
	// Dials system-probe
	conn, err := grpc.Dial(systemProbeAddr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	return &RuntimeSecurityAgent{
		conn: conn,
		wg:   &sync.WaitGroup{},
		logClient: NewDDClientWithLogSource(config.NewLogSource(logSource, &config.LogsConfig{
			Type:    logType,
			Service: logService,
			Source:  logSource,
		})),
	}, nil
}

// Start - Starts the Runtime Security agent
func (rsa *RuntimeSecurityAgent) Start() error {
	// Start the Datadog log client. This client is used to ship security events to Datadog.
	go rsa.logClient.Run(rsa.wg)
	// Start the system-probe events listener
	go rsa.StartEventListener()
	return nil
}

// Stop - Stops the Runtime Security agent
func (rsa *RuntimeSecurityAgent) Stop() error {
	rsa.running.Store(false)
	rsa.logClient.Stop()
	rsa.wg.Wait()
	rsa.conn.Close()
	return nil
}

// StartEventListener - Listens for new events from system-probe
func (rsa *RuntimeSecurityAgent) StartEventListener() {
	rsa.wg.Add(1)
	defer rsa.wg.Done()
	apiClient := api.NewSecurityModuleClient(rsa.conn)

	rsa.running.Store(true)
	for rsa.running.Load() == true {
		stream, err := apiClient.GetEvents(context.Background(), &api.GetParams{})
		if err != nil {
			log.Errorf("grpc stream connection error: %v", err)
			// retry in 2 seconds
			time.Sleep(2*time.Second)
			continue
		}

		for {
			// Get new event from stream
			in, err := stream.Recv()
			if err == io.EOF || in == nil {
				break
			}
			log.Infof("Got message from rule `%s` for event `%s` with tags `%+v` ", in.RuleID, string(in.Data), in.Tags)

			// Dispatch security event
			rsa.DispatchEvent(in)
		}
	}
}

// DispatchEvent - Dispatches a security event message to the subsytems of the runtime security agent
func (rsa *RuntimeSecurityAgent) DispatchEvent(evt *api.SecurityEventMessage) {
	// For now simply log to Datadog
	rsa.logClient.SendSecurityEvent(evt, message.StatusAlert)
}
