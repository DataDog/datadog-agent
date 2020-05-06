package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"

	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type DDLog struct {
	Host    string          `json:"host"`
	Service string          `json:"service"`
	Rule    string          `json:rule`
	Event   json.RawMessage `json:"event"`
}

type EventClient struct {
	conn    *grpc.ClientConn
	running atomic.Value
	wg      sync.WaitGroup
}

func (c *EventClient) Start() {
	c.wg.Add(1)
	defer c.wg.Done()

	apiClient := api.NewSecurityModuleClient(c.conn)

	c.running.Store(true)
	for c.running.Load() == true {
		for {
			stream, err := apiClient.GetEvents(context.Background(), &api.GetParams{})
			if err != nil {
				panic(err)
			}

			in, err := stream.Recv()
			if err == io.EOF || in == nil {
				break
			}

			log.Debugf("Got message from rule %s with event %s", in.RuleName, string(in.Data))

			ddlog := DDLog{
				Host:    "my.vagrant",
				Service: "security-agent",
				Rule:    in.RuleName,
				Event:   in.Data,
			}

			d, err := json.Marshal(ddlog)
			if err != nil {
				panic(err)
			}

			apiKey := os.Getenv("DD_API_KEY")

			_, err = http.Post("https://http-intake.logs.datadoghq.com/v1/input/"+apiKey, "application/json", bytes.NewBuffer(d))
			if err != nil {
				log.Error(err)
			}
		}
	}

}

func (c *EventClient) Stop() {
	c.running.Store(false)
	c.wg.Wait()
	c.conn.Close()
}

func NewEventClient() (*EventClient, error) {
	conn, err := grpc.Dial("localhost:8787", grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	return &EventClient{
		conn: conn,
	}, nil
}
