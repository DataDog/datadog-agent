package module

import (
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

type EventServer struct {
	msgs chan *api.SecurityEventMessage
}

func (e *EventServer) GetEvents(params *api.GetParams, stream api.SecurityModule_GetEventsServer) error {
	msgs := 10
LOOP:
	for {
		select {
		case msg := <-e.msgs:
			stream.Send(msg)
			msgs--
		case <-time.After(time.Second):
			break LOOP
		}

		if msgs <= 0 {
			break
		}
	}

	return nil
}

func (e *EventServer) DiscriminatorDiscovered(event eval.Event, field string) {
}

func (e *EventServer) RuleMatch(rule *eval.Rule, event eval.Event) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	msg := &api.SecurityEventMessage{
		RuleName: rule.Name,
		Data:     data,
	}

	e.msgs <- msg
}

func NewEventServer() *EventServer {
	return &EventServer{
		msgs: make(chan *api.SecurityEventMessage, 1000),
	}
}
