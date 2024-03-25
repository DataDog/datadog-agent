package dataobs

import (
	"fmt"
	"time"

	"github.com/xeipuuv/gojsonschema"
)

type OpenLineageState struct {
	startTimes map[string]time.Time
}

func NewOpenLineageState() *OpenLineageState {
	return &OpenLineageState{
		startTimes: make(map[string]time.Time),
	}
}

// const (
// 	StartEventType    = "START"
// 	RunningEventType  = "RUNNING"
// 	CompleteEventType = "COMPLETE"
// 	AbortEventType    = "ABORT"
// 	FailedEventType   = "FAIL"
// 	OtherEventType    = "OTHER"
// )

var schemaLoader gojsonschema.JSONLoader = gojsonschema.NewReferenceLoader("https://openlineage.io/spec/2-0-2/OpenLineage.json")

func EventToLog(event []byte) error {

	fmt.Println("new event ", string(event))
	result, err := gojsonschema.Validate(schemaLoader, gojsonschema.NewBytesLoader(event))

	if err != nil {
		panic(err.Error())
	}

	if result.Valid() {
		fmt.Printf("The OL event is valid\n")

	} else {
		fmt.Printf("The OL event is not valid. see errors :\n")
		for _, desc := range result.Errors() {
			fmt.Printf("- %s\n", desc)
		}
	}
	return err
}
