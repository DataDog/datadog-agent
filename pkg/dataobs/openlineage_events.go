package dataobs

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/xeipuuv/gojsonschema"
)

type OpenLineage struct {
	forwarder eventplatform.Forwarder
}

func NewOpenLineage(forwarder eventplatform.Forwarder) *OpenLineage {
	return &OpenLineage{
		forwarder: forwarder,
	}
}

var schemaLoader gojsonschema.JSONLoader = gojsonschema.NewReferenceLoader("https://openlineage.io/spec/2-0-2/OpenLineage.json")

func (openLineage *OpenLineage) EventToLog(event []byte) (string, error) {
	fmt.Println("new event ", string(event))
	result, err := gojsonschema.Validate(schemaLoader, gojsonschema.NewBytesLoader(event))

	if result.Valid() {
		fmt.Printf("The OL event is valid\n")
		m := message.NewMessage(event, nil, "", 0)
		e := openLineage.forwarder.SendEventPlatformEvent(m, eventplatform.EventTypeDataobs)
		if e != nil {
			fmt.Printf("Error while sending event to event platform: %v\n", e)
		}
		return string(event), nil

	} else {
		fmt.Printf("The OL event is not valid. see errors :\n")
		for _, desc := range result.Errors() {
			fmt.Printf("- %s\n", desc)
		}
		return "", err
	}
}
