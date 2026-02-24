package functiontools

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"

	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/jsonapi"
	"github.com/mitchellh/mapstructure"
)

type Params struct {
	Kind        reflect.Kind `json:"kind"`
	Description string       `json:"description"`
}

type FunctionTool struct {
	description string
	params      map[string]Params
	handler     func(parameters map[string]any) (any, error)
}

var registry = map[string]FunctionTool{
	"list_files": {
		description: "Recursively list all files under a directory that end with a given extension (e.g. 'log' or 'txt'). Returns full file paths. Commonly used to inspect logs located under '/var/log'.",
		params: map[string]Params{
			"directory": {Kind: reflect.String, Description: "The directory to search from (search is recursive). For system logs, this is typically '/var/log'."},
			"extension": {Kind: reflect.String, Description: "The file extension to match, exclusing the dot, e.g. 'log' or 'txt'."},
		},
		handler: func(parameters map[string]any) (any, error) {
			return listFiles(parameters["directory"].(string), parameters["extension"].(string))
		},
	},
	"tail_file": {
		description: "Read the last N lines of a text file (such as a log file).",
		params: map[string]Params{
			"file_path": {Kind: reflect.String, Description: "The full path of the file to tail."},
			"n_lines":   {Kind: reflect.Float64, Description: "The number of lines to read from the end of the file. Usually 10 lines is enough to get the last events."},
		},
		handler: func(parameters map[string]any) (any, error) {
			return tailFile(parameters["file_path"].(string), parameters["n_lines"].(float64))
		},
	},
	"enable_ssi": {
		description: "Enable Single Step Instrumentation (SSI) for a given application in the cluster.",
		params: map[string]Params{
			"namespace":       {Kind: reflect.String, Description: "The namespace of the application to enable SSI for."},
			"deployment_name": {Kind: reflect.String, Description: "The name of the deployment to enable SSI for."},
			"tracer_versions": {Kind: reflect.String, Description: "The language and version of the tracers to enable for the application, separated by a colon. For example: `python:3,java:1`."},
		},
		handler: func(parameters map[string]any) (any, error) {
			return enableSsi(parameters["namespace"].(string), parameters["deployment_name"].(string), parameters["tracer_versions"].(string))
		},
	},
	"list_pods": {
		description: "List all Kubernetes pods across all namespaces, grouped by namespace.",
		params:      map[string]Params{},
		handler: func(parameters map[string]any) (any, error) {
			return listPods()
		},
	},
	"delete_pod": {
		description: "Delete a Kubernetes pod by name and namespace.",
		params: map[string]Params{
			"namespace": {Kind: reflect.String, Description: "The namespace of the pod to delete."},
			"pod_name":  {Kind: reflect.String, Description: "The name of the pod to delete."},
		},
		handler: func(parameters map[string]any) (any, error) {
			return deletePod(parameters["namespace"].(string), parameters["pod_name"].(string))
		},
	},
}

type Tool struct {
	Name   string         `json:"name" jsonapi:"primary,name"`
	Params map[string]any `json:"params" jsonapi:"attr,params"`
}
type ToolCall struct {
	TaskID string `json:"task_id" jsonapi:"primary,task"`
	DDAKey string `json:"datadog_agent_key" jsonapi:"attr,datadog_agent_key"`
	Tool   *Tool  `json:"tool" jsonapi:"attr,tool"`

	Output any    `json:"output" jsonapi:"attr,output"`
	Error  string `json:"error,omitempty" jsonapi:"attr,error"`
}

func NewCall(task types.AgentTaskConfig) ToolCall {
	taskIDRaw, ok := task.Config.TaskArgs["task_id"]
	if !ok {
		return ToolCall{
			Error: "no task id provided",
		}
	}
	taskID, ok := taskIDRaw.(string)
	if !ok {
		return ToolCall{
			Error: "task id is not a string",
		}
	}

	datadogAgentKeyRaw, ok := task.Config.TaskArgs["datadog_agent_key"]
	if !ok {
		return ToolCall{
			Error: "no datadog agent key provided",
		}
	}
	datadogAgentKey, ok := datadogAgentKeyRaw.(string)
	if !ok {
		return ToolCall{
			Error: "datadog agent key is not a string",
		}
	}

	raw, ok := task.Config.TaskArgs["tool"]
	if !ok {
		return ToolCall{
			Error: "no tool provided",
		}
	}

	// The raw value is map[string]interface{} from JSON unmarshaling,
	// so we use mapstructure to decode it into the Tool struct
	var tool Tool
	if err := mapstructure.Decode(raw, &tool); err != nil {
		return ToolCall{
			Error: "tool is not a valid tool: " + err.Error(),
		}
	}

	return ToolCall{
		TaskID: taskID,
		DDAKey: datadogAgentKey,
		Tool:   &tool,
	}
}

func (toolCall ToolCall) Execute() ToolCall {
	if toolCall.Error != "" {
		toolCall.Error = "failed to execute: " + toolCall.Error
		return toolCall
	}

	functionTool, ok := registry[toolCall.Tool.Name]
	if !ok {
		toolCall.Error = fmt.Sprintf("tool '%s' not found", toolCall.Tool.Name)
		return toolCall
	}

	if len(registry[toolCall.Tool.Name].params) != len(toolCall.Tool.Params) {
		toolCall.Error = fmt.Sprintf("expected '%d' parameters, got '%d'", len(registry[toolCall.Tool.Name].params), len(toolCall.Tool.Params))
		return toolCall
	}

	for registryName, registryParam := range registry[toolCall.Tool.Name].params {
		callParam, ok := toolCall.Tool.Params[registryName]
		if !ok {
			toolCall.Error = fmt.Sprintf("parameter '%s' is required", registryName)
			return toolCall
		}

		callParamKind := reflect.TypeOf(callParam).Kind()
		if callParamKind != registryParam.Kind {
			toolCall.Error = fmt.Sprintf("parameter '%s' should be of kind '%s', got '%s'", registryName, registryParam.Kind, callParamKind)
			return toolCall
		}
	}

	result, err := functionTool.handler(toolCall.Tool.Params)
	if err != nil {
		toolCall.Error = err.Error()
		return toolCall
	}

	toolCall.Output = result
	return toolCall
}

func (toolCall ToolCall) Send() error {
	if toolCall.Error != "" {
		return fmt.Errorf("failed to send result: %s", toolCall.Error)
	}
	if toolCall.Output == nil {
		return fmt.Errorf("output is empty")
	}
	jsonData, err := jsonapi.Marshal(toolCall)
	fmt.Println("jsonData: " + string(jsonData))
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Send the payload to the Datadog intake API as a custom event (example endpoint)
	datadogAPIKey := os.Getenv("DD_API_KEY")
	if datadogAPIKey == "" {
		return fmt.Errorf("Datadog API key not set. Please set the DD_API_KEY environment variable")
	}

	req, err := http.NewRequest(
		"POST",
		"https://app.datad0g.com/api/unstable/dda-ft-api/task/finish?force_tracing=1",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", datadogAPIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status code: '%d' with body: %s", resp.StatusCode, respBody)
	}

	fmt.Printf("Successfully sent result to webserver (status: %d)\n", resp.StatusCode)
	return nil
}
