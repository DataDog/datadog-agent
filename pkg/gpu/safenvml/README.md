# SafeNVML Package

The `safenvml` package provides a safe wrapper around NVIDIA's NVML library. It ensures compatibility with older drivers by checking symbol availability and prevents runtime panics when using NVML functions that might not be available in all driver versions.

## Adding a New NVML API

When adding a new NVML API function to this package, follow these steps to ensure proper implementation and error handling:

### 1. Add the function signature to the appropriate interface

Depending on whether the function is related to the NVML library itself or to a specific device, add it to either:
- `SafeNVML` interface in `lib.go` for general NVML functions
- `SafeDevice` interface in `device.go` for device-specific functions

Make sure to include proper documentation for the function.

Example:
```go
// SafeDevice interface
type SafeDevice interface {
    // ... existing methods ...

    // GetNewMetric returns some new metric from the device
    GetNewMetric() (uint32, error)
}
```

### 2. Add the function to the API registry

Add the function name to either `getCriticalAPIs()` or `getNonCriticalAPIs()` in `lib.go`:

- Use `getCriticalAPIs()` for essential functions that should cause initialization to fail if not available
- Use `getNonCriticalAPIs()` for optional functions that are nice to have but not critical

For device methods, use `toNativeName()` to convert to the NVML naming convention.

Example:
```go
func getNonCriticalAPIs() []string {
    return []string{
        // ... existing APIs ...
        toNativeName("GetNewMetric"),  // For device functions
        "nvmlSomeNewFunction",         // For library functions
    }
}
```

### 3. Implement the function

Implement the function in the appropriate type:
- For `SafeNVML` functions, implement in `safeNvml` struct in `lib.go`
- For `SafeDevice` functions, implement in `safeDeviceImpl` struct in `device_impl.go`

Make sure to:
1. Check if the function is available using the `lookup()` method
2. Call the underlying NVML function if available
3. Wrap any NVML errors properly

Example implementation for a device function:
```go
func (d *safeDeviceImpl) GetNewMetric() (uint32, error) {
    if err := d.lib.lookup(toNativeName("GetNewMetric")); err != nil {
        return 0, err
    }
    value, ret := d.nvmlDevice.GetNewMetric()
    return value, NewNvmlAPIErrorOrNil("GetNewMetric", ret)
}
```

## Testing

When adding a new API, the test infrastructure will automatically include it in the mock capabilities. No additional changes are needed in `lib_mock.go` as it references the same API registry functions.
