package module

import (
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

func (e *EventServer) SendEvent(rule *eval.Rule, event eval.Event) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	log.Infof("Sending event message for rule `%s` to security-agent `%s` with tags %v", rule.ID, string(data), rule.Tags)

	msg := &api.SecurityEventMessage{
		RuleID: rule.ID,
		Type:   event.GetType(),
		Tags:   rule.Tags,
		Data:   data,
	}

	select {
	case e.msgs <- msg:
		break
	default:
		// Do not wait for the channel to free up, we don't want to delay the processing pipeline further
		log.Warnf("the event server channel is full, an event of ID %v was dropped", msg.RuleID)
		break
	}
}

func NewEventServer() *EventServer {
	return &EventServer{
		msgs: make(chan *api.SecurityEventMessage, 1000),
	}
}
