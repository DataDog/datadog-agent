// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"errors"
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// NvmlAPIError represents an error when interacting with the NVML API.
// It wraps nvml.Return values to provide idiomatic error handling in Go.
type NvmlAPIError struct {
	// APIName is the name of the API that failed
	APIName string
	// NvmlErrorCode is the NVML error code
	NvmlErrorCode nvml.Return
}

// Error implements the error interface
func (e *NvmlAPIError) Error() string {
	switch {
	case errors.Is(e.NvmlErrorCode, nvml.ERROR_FUNCTION_NOT_FOUND):
		return e.APIName + " symbol not found in NVML library"
	case errors.Is(e.NvmlErrorCode, nvml.ERROR_NOT_SUPPORTED):
		return e.APIName + " is not supported by the GPU or driver"
	default:
		return fmt.Sprintf("NVML API error for %s: %s", e.APIName, nvml.ErrorString(e.NvmlErrorCode))
	}
}

// NewNvmlAPIErrorOrNil creates a new NvmlAPIError with the given API name and error code,
// or returns nil if the error code is nvml.SUCCESS
func NewNvmlAPIErrorOrNil(apiName string, errorCode nvml.Return) error {
	if errors.Is(errorCode, nvml.SUCCESS) {
		return nil
	}
	return &NvmlAPIError{
		APIName:       apiName,
		NvmlErrorCode: errorCode,
	}
}

// IsUnsupported checks if an error indicates that the device doesn't support a particular API
// This is indicated by ERROR_NOT_SUPPORTED or ERROR_FUNCTION_NOT_FOUND error codes
func IsUnsupported(err error) bool {
	var nvmlErr *NvmlAPIError
	return err != nil && errors.As(err, &nvmlErr) &&
		(errors.Is(nvmlErr.NvmlErrorCode, nvml.ERROR_NOT_SUPPORTED) ||
			errors.Is(nvmlErr.NvmlErrorCode, nvml.ERROR_FUNCTION_NOT_FOUND))
}

// IsTimeout checks if an error indicates that no event arrived in specified timeout or that an interrupt arrived
func IsTimeout(err error) bool {
	var nvmlErr *NvmlAPIError
	return err != nil && errors.As(err, &nvmlErr) && errors.Is(nvmlErr.NvmlErrorCode, nvml.ERROR_TIMEOUT)
}

// IsDriverNotLoaded checks if an error indicates that the driver is not loaded
func IsDriverNotLoaded(err error) bool {
	var nvmlErr *NvmlAPIError
	return err != nil && errors.As(err, &nvmlErr) && errors.Is(nvmlErr.NvmlErrorCode, nvml.ERROR_DRIVER_NOT_LOADED)
}

func IsInvalidArgument(err error) bool {
	var nvmlErr *NvmlAPIError
	return err != nil && errors.As(err, &nvmlErr) && errors.Is(nvmlErr.NvmlErrorCode, nvml.ERROR_INVALID_ARGUMENT)
}

// IsAPIUnsupportedOnDevice checks if an error indicates that the API is not supported on the device.
// Requires the device because some error codes indicate unsupported APIs when the device is MIG.
func IsAPIUnsupportedOnDevice(err error, device Device) bool {
	_, isMig := device.(*MIGDevice)
	return IsUnsupported(err) || (isMig && IsInvalidArgument(err))
}
