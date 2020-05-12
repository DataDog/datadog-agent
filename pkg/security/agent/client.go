package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type DDLog struct {
	Host    string          `json:"host"`
	Service string          `json:"service"`
	Source  string          `json:"ddsource"`
	Tags    string          `json:"ddtags"`
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

			log.Infof("Got message from rule `%s` for event `%s` with tags `%+v` ", in.RuleID, string(in.Data), in.Tags)

			ddlog := DDLog{
				Host:    "my.vagrant",
				Service: "security-agent",
				Source:  in.Type,
				Rule:    in.RuleID,
				Event:   in.Data,
				Tags:    strings.Join(in.Tags, ","),
			}

			d, err := json.Marshal(ddlog)
			if err != nil {
				panic(err)
			}

			apiKey := os.Getenv("DD_API_KEY")

			url := fmt.Sprintf("https://http-intake.logs.datadoghq.com/v1/input/%s", apiKey)
			resp, err := http.Post(url, "application/json", bytes.NewBuffer(d))
			if err != nil {
				log.Error(err)
			}
			log.Infof("Log sent, response: %+v\n", resp)
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
