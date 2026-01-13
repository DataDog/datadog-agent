package functiontools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
)

type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type Parameters struct {
	Type                 string              `json:"type"`
	Properties           map[string]Property `json:"properties"`
	Required             []string            `json:"required"`
	AdditionalProperties bool                `json:"additionalProperties"`
}

type Description struct {
	Type        string     `json:"type"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Strict      bool       `json:"strict"`
	Parameters  Parameters `json:"parameters"`
}

type FunctionTool struct {
	description string
	properties  map[string]Property
	handler     func(parameters map[string]string) (any, error)
}

var registry = map[string]FunctionTool{
	"list_files": {
		description: "Recursively list all files under a directory that end with a given extension (e.g. 'log' or 'txt'). Returns full file paths. Commonly used to inspect logs located under '/var/log'.",
		properties: map[string]Property{
			"directory": {Type: "string", Description: "The directory to search from (search is recursive). For system logs, this is typically '/var/log'."},
			"extension": {Type: "string", Description: "The file extension to match, exclusing the dot, e.g. 'log' or 'txt'."},
		},
		handler: func(parameters map[string]string) (any, error) {
			return listFiles(parameters["directory"], parameters["extension"])
		},
	},
	"tail_file": {
		description: "Read the last N lines of a text file (such as a log file).",
		properties: map[string]Property{
			"file_path": {Type: "string", Description: "The full path of the file to tail."},
			"n_lines":   {Type: "integer", Description: "The number of lines to read from the end of the file. Usually 10 lines is enough to get the last events."},
		},
		handler: func(parameters map[string]string) (any, error) {
			return tailFile(parameters["file_path"], parameters["n_lines"])
		},
	},
	"enable_ssi": {
		description: "Enable Single Step Instrumentation (SSI) for a given application in the cluster.",
		properties: map[string]Property{
			"namespace":       {Type: "string", Description: "The namespace of the application to enable SSI for."},
			"deployment_name": {Type: "string", Description: "The name of the deployment to enable SSI for."},
			"tracer_versions": {Type: "string", Description: "The language and version of the tracers to enable for the application, separated by a colon. For example: `python:3,java:1`."},
		},
		handler: func(parameters map[string]string) (any, error) {
			return enableSsi(parameters["namespace"], parameters["deployment_name"], parameters["tracer_versions"])
		},
	},
	"list_pods": {
		description: "List all Kubernetes pods across all namespaces, grouped by namespace.",
		properties:  map[string]Property{},
		handler: func(parameters map[string]string) (any, error) {
			return listPods()
		},
	},
	"delete_pod": {
		description: "Delete a Kubernetes pod by name and namespace.",
		properties: map[string]Property{
			"namespace": {Type: "string", Description: "The namespace of the pod to delete."},
			"pod_name":  {Type: "string", Description: "The name of the pod to delete."},
		},
		handler: func(parameters map[string]string) (any, error) {
			return deletePod(parameters["namespace"], parameters["pod_name"])
		},
	},
}

type FunctionToolCall struct {
	CallID           string            `json:"call_id"`
	DatadogAgentKey  string            `json:"datadog_agent_key"`
	FunctionToolName string            `json:"function_tool_name"`
	Parameters       map[string]string `json:"parameters"`
	Output           any               `json:"output"`
	Error            error             `json:"error,omitempty"`
}

func NewCall(task types.AgentTaskConfig) FunctionToolCall {
	raw, ok := task.Config.TaskArgs["parameters"]
	if !ok {
		// No parameters provided, return an empty map (not nil)
		return FunctionToolCall{
			CallID:           task.Config.TaskArgs["call_id"],
			FunctionToolName: task.Config.TaskArgs["function_tool_name"],
			Parameters:       make(map[string]string),
		}
	}

	var parameters map[string]string
	err := json.Unmarshal([]byte(raw), &parameters)
	if err != nil {
		return FunctionToolCall{
			Error: fmt.Errorf("failed to unmarshal parameters: %w", err),
		}
	}
	return FunctionToolCall{
		CallID:           task.Config.TaskArgs["call_id"],
		FunctionToolName: task.Config.TaskArgs["function_tool_name"],
		Parameters:       parameters,
	}
}

func (functionToolCall FunctionToolCall) Execute() FunctionToolCall {
	if functionToolCall.Error != nil {
		return FunctionToolCall{
			Error: fmt.Errorf("failed to execute: %w", functionToolCall.Error),
		}
	}
	functionTool, ok := registry[functionToolCall.FunctionToolName]
	if !ok {
		functionToolCall.Error = fmt.Errorf("function tool '%s' not found", functionToolCall.FunctionToolName)
		return functionToolCall
	}
	if len(registry[functionToolCall.FunctionToolName].properties) != len(functionToolCall.Parameters) {
		functionToolCall.Error = fmt.Errorf("expected %d parameters, got %d", len(registry[functionToolCall.FunctionToolName].properties), len(functionToolCall.Parameters))
		return functionToolCall
	}
	for name := range registry[functionToolCall.FunctionToolName].properties {
		if _, ok := functionToolCall.Parameters[name]; !ok {
			functionToolCall.Error = fmt.Errorf("parameter '%s' is required", name)
			return functionToolCall
		}
	}
	result, err := functionTool.handler(functionToolCall.Parameters)
	if err != nil {
		functionToolCall.Error = err
		return functionToolCall
	}
	functionToolCall.Output = result
	return functionToolCall
}

func (functionToolCall FunctionToolCall) Send() error {
	if functionToolCall.Error != nil {
		return fmt.Errorf("failed to send result: %w", functionToolCall.Error)
	}
	if functionToolCall.Output == nil {
		return fmt.Errorf("output is empty")
	}
	jsonData, err := json.Marshal(functionToolCall)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	resp, err := http.Post(
		"http://host.minikube.internal:8123/functiontool",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	fmt.Printf("Successfully sent result to webserver (status: %d)\n", resp.StatusCode)
	return nil
}
