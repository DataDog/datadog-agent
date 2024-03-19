package openlineage

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

const (
	StartEventType    = "START"
	RunningEventType  = "RUNNING"
	CompleteEventType = "COMPLETE"
	AbortEventType    = "ABORT"
	FailedEventType   = "FAIL"
	OtherEventType    = "OTHER"
)

type State struct {
	startTimes map[string]time.Time
}

func NewState() *State {
	return &State{
		startTimes: make(map[string]time.Time),
	}
}

// flatten recursively traverses the input data map, flattens nested maps, and populates the meta map
func flatten(prefix string, data map[string]interface{}, meta map[string]string) {
	for key, value := range data {
		// If value is a nested map, recursively flatten it
		if nestedMap, ok := value.(map[string]interface{}); ok {
			flatten(prefix+key+".", nestedMap, meta)
			continue
		}
		if nestedArray, ok := value.([]interface{}); ok {
			data, err := json.Marshal(nestedArray)
			if err == nil {
				meta[prefix+key] = string(data)
			} else {
				fmt.Printf("Error marshalling array: %v", err)
			}
			continue
		}
		// Convert value to string and add to meta map
		meta[prefix+key] = fmt.Sprintf("%v", value)
	}
}

func (s *State) EventToSpan(event []byte) (*pb.TracerPayload, error) {
	fmt.Println("new event ", string(event))
	var data map[string]interface{}
	err := json.Unmarshal(event, &data)
	if err != nil {
		fmt.Println("Error:", err)
		return nil, fmt.Errorf("error unmarshalling: %v", err)
	}
	eventType, ok := data["eventType"].(string)
	if !ok {
		return nil, fmt.Errorf("missing eventType field")
	}
	eventTime, ok := data["eventTime"].(string)
	if !ok {
		return nil, fmt.Errorf("missing eventTime field")
	}
	start, err := time.Parse(time.RFC3339Nano, eventTime)
	if err != nil {
		fmt.Println("Error:", err)
		return nil, fmt.Errorf("error parsing eventTime: %v", err)
	}
	duration := time.Duration(0)
	run, ok := data["run"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing run field")
	}
	runID, ok := run["runId"].(string)
	if !ok {
		return nil, fmt.Errorf("missing runId field")
	}
	job, ok := data["job"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing job field")
	}
	jobName, ok := job["name"].(string)
	if !ok {
		return nil, fmt.Errorf("missing name field")
	}
	jobNamespace, ok := job["namespace"].(string)
	if !ok {
		return nil, fmt.Errorf("missing namespace field")
	}
	if eventType == StartEventType {
		s.startTimes[runID] = start
	}
	if eventType == CompleteEventType {
		startTime, ok := s.startTimes[runID]
		if ok {
			duration = start.Sub(startTime)
			start = startTime
		}
	}
	serviceName := fmt.Sprintf("%s.%s", jobNamespace, jobName)
	hasher := fnv.New64a()
	hasher.Write([]byte(runID))
	traceID := hasher.Sum64()
	operationName := "run"
	meta := map[string]string{}
	flatten("", data, meta)
	return &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{
			{
				Priority: 1,
				Spans: []*pb.Span{
					{
						Service:  serviceName,
						Meta:     meta,
						TraceID:  traceID,
						SpanID:   traceID,
						Name:     operationName,
						Resource: eventType,
						Start:    start.UnixNano(),
						Duration: duration.Nanoseconds(),
					},
				},
			},
		},
	}, nil
}
