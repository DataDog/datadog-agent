// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Some parts of this file are taken from : https://github.com/aws-samples/aws-lambda-extensions/tree/main/go-example-extension

package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/DataDog/agent-payload/v5/gogen"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

const extensionName = "recorder-extension" // extension name has to match the filename
var extensionClient = NewClient(os.Getenv("AWS_LAMBDA_RUNTIME_API"))
var nbHitMetrics = 0
var nbReport = 0
var nbHitTraces = 0
var outputSketches = make([]gogen.SketchPayload_Sketch, 0)
var outputLogs = make([]jsonServerlessPayload, 0)
var outputTraces = make([]*pb.TracerPayload, 0)
var hasBeenOutput = false

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigs
		cancel()
	}()

	err := extensionClient.Register(ctx, extensionName)
	if err != nil {
		panic(err)
	}

	// port 8080 is used by the Lambda Invoke API
	port := "3333"
	Start(port)

	// Will block until shutdown event is received or cancelled via the context.
	processEvents(ctx)
}

func processEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			res, err := extensionClient.NextEvent(ctx)
			if err != nil {
				return
			}
			if res.EventType == Shutdown {
				time.Sleep(1900 * time.Millisecond)
				return
			}
		}
	}
}

// JSON representation of a message.
type jsonServerlessPayload struct {
	Message   jsonServerlessMessage `json:"message"`
	Status    string                `json:"status"`
	Timestamp int64                 `json:"timestamp"`
	Hostname  string                `json:"hostname"`
	Service   string                `json:"service"`
	Source    string                `json:"ddsource"`
	Tags      string                `json:"ddtags"`
}

type jsonServerlessMessage struct {
	Message string                `json:"message"`
	Lambda  *jsonServerlessLambda `json:"lambda,omitempty"`
}

type jsonServerlessLambda struct {
	ARN       string `json:"arn"`
	RequestID string `json:"request_id,omitempty"`
}

// NextEventResponse is the response for /event/next
type NextEventResponse struct {
	EventType EventType `json:"eventType"`
}

// EventType represents the type of events recieved from /event/next
type EventType string

const (
	// Shutdown is a shutdown event for the environment
	Shutdown EventType = "SHUTDOWN"

	extensionNameHeader      = "Lambda-Extension-Name"
	extensionIdentiferHeader = "Lambda-Extension-Identifier"
)

// Client is a simple client for the Lambda Extensions API
type Client struct {
	baseURL     string
	httpClient  *http.Client
	extensionID string
}

// NewClient returns a Lambda Extensions API client
func NewClient(awsLambdaRuntimeAPI string) *Client {
	baseURL := fmt.Sprintf("http://%s/2020-01-01/extension", awsLambdaRuntimeAPI)
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

// Register will register the extension with the Extensions API
func (e *Client) Register(ctx context.Context, filename string) error {
	const action = "/register"
	url := e.baseURL + action

	reqBody, err := json.Marshal(map[string]interface{}{
		"events": []EventType{Shutdown},
	})
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}
	httpReq.Header.Set(extensionNameHeader, filename)
	httpRes, err := e.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	if httpRes.StatusCode != 200 {
		return fmt.Errorf("request failed with status %s", httpRes.Status)
	}
	defer httpRes.Body.Close()
	e.extensionID = httpRes.Header.Get(extensionIdentiferHeader)
	return nil
}

// NextEvent blocks while long polling for the next lambda invoke or shutdown
func (e *Client) NextEvent(ctx context.Context) (*NextEventResponse, error) {
	const action = "/event/next"
	url := e.baseURL + action

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set(extensionIdentiferHeader, e.extensionID)
	httpRes, err := e.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if httpRes.StatusCode != 200 {
		return nil, fmt.Errorf("request failed with status %s", httpRes.Status)
	}
	defer httpRes.Body.Close()
	body, err := io.ReadAll(httpRes.Body)
	if err != nil {
		return nil, err
	}
	res := NextEventResponse{}
	err = json.Unmarshal(body, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// Start is starting the http server to receive logs, traces and metrics
func Start(port string) {
	go startHTTPServer(port)
}

func startHTTPServer(port string) {
	http.HandleFunc("/api/beta/sketches", func(w http.ResponseWriter, r *http.Request) {
		nbHitMetrics++
		body, err := io.ReadAll(r.Body)
		if err != nil {
			fmt.Printf("Error while reading HTTP request body: %s \n", err)
			return
		}
		pl := new(gogen.SketchPayload)
		if err := pl.Unmarshal(body); err != nil {
			fmt.Printf("Error while unmarshalling sketches %s \n", err)
			return
		}

		outputSketches = append(outputSketches, pl.Sketches...)

		if nbHitMetrics == 3 {
			// two calls + shutdown
			jsonSketch, err := json.Marshal(outputSketches)
			if err != nil {
				fmt.Printf("Error while JSON encoding the sketch")
			}
			fmt.Printf("%s%s%s\n", "BEGINMETRIC", string(jsonSketch), "ENDMETRIC")
		}
	})

	http.HandleFunc("/api/v2/logs", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return
		}
		decompressedBody, err := decompress(body)
		if err != nil {
			return
		}
		var messages []jsonServerlessPayload
		if err := json.Unmarshal(decompressedBody, &messages); err != nil {
			return
		}

		for _, log := range messages {
			msg := log.Message.Message
			if strings.Contains(msg, "BEGINMETRIC") || strings.Contains(msg, "BEGINLOG") || strings.Contains(msg, "BEGINTRACE") {
				continue
			}
			if strings.HasPrefix(msg, "REPORT RequestId:") {
				nbReport++
			}
			outputLogs = append(outputLogs, log)
		}

		if nbReport == 2 && !hasBeenOutput {
			jsonLogs, err := json.Marshal(outputLogs)
			if err != nil {
				fmt.Printf("Error while JSON encoding the logs")
			}
			fmt.Printf("%s%s%s\n", "BEGINLOG", string(jsonLogs), "ENDLOG")
			hasBeenOutput = true // make sure not re re-output the logs
		}

	})

	http.HandleFunc("/api/v0.2/traces", func(w http.ResponseWriter, r *http.Request) {
		nbHitTraces++
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return
		}
		decompressedBody, err := decompress(body)
		if err != nil {
			return
		}
		pl := new(pb.AgentPayload)
		if err := pl.Unmarshal(decompressedBody); err != nil {
			fmt.Printf("Error while unmarshalling traces %s \n", err)
			return
		}

		outputTraces = append(outputTraces, pl.TracerPayloads...)

		if nbHitTraces == 2 {
			jsonLogs, err := json.Marshal(outputTraces)
			if err != nil {
				fmt.Printf("Error while JSON encoding the traces")
			}
			fmt.Printf("%s%s%s\n", "BEGINTRACE", string(jsonLogs), "ENDTRACE")
		}
	})

	http.HandleFunc("/api/v1/series", func(w http.ResponseWriter, r *http.Request) {
	})

	http.HandleFunc("/api/v1/check_run", func(w http.ResponseWriter, r *http.Request) {
	})

	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		panic(err)
	}
}

func decompress(payload []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	_, err = buffer.ReadFrom(reader)
	if err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}
