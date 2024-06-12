package autoinstrumentation

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	// defaultMilliCPURequest defines default milli cpu request number.
	defaultMilliCPURequest int64 = 50 // 0.05 core

	// defaultMemoryRequest defines default memory request size.
	defaultMemoryRequest int64 = 20 * 1024 * 1024 // 20 MB
)

var (
	defaultCPUResource = resource.NewMilliQuantity(
		defaultMilliCPURequest,
		resource.DecimalSI,
	)

	defaultMemoryResource = resource.NewMilliQuantity(
		defaultMemoryRequest,
		resource.DecimalSI,
	)
)

func resourceQuantityFromConfig(
	configKey string,
	name v1.ResourceName,
	orDefault *resource.Quantity,
) (v1.ResourceName, resource.Quantity, error) {
	if !config.Datadog().IsSet(configKey) {
		if orDefault == nil {
			return name, resource.Quantity{}, fmt.Errorf("default resource quantity missing for %q", name)
		}

		return name, *orDefault, nil
	}

	q, err := resource.ParseQuantity(config.Datadog().GetString(configKey))
	if err != nil {
		return name, resource.Quantity{}, err
	}

	return name, q, nil
}

func initContainerResourceRequirements() (v1.ResourceRequirements, error) {
	rMemory, memory, err := resourceQuantityFromConfig(
		"admission_controller.auto_instrumentation.init_resources.memory",
		v1.ResourceMemory, defaultMemoryResource,
	)
	if err != nil {
		var r v1.ResourceRequirements
		return r, err
	}

	rCpu, cpu, err := resourceQuantityFromConfig(
		"admission_controller.auto_instrumentation.init_resources.cpu",
		v1.ResourceCPU, defaultCPUResource,
	)
	if err != nil {
		var r v1.ResourceRequirements
		return r, err
	}

	return v1.ResourceRequirements{
		Limits:   v1.ResourceList{rMemory: memory, rCpu: cpu},
		Requests: v1.ResourceList{rMemory: memory, rCpu: cpu},
	}, nil
}
