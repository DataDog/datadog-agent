package runtime

// Telemetry stores telemetry related to a particular type of runtime asset
type Telemetry interface {
	Get() map[string]int64
}

type noOpTelemetry struct{}
func newNoOpTelemetry() Telemetry {
	return &noOpTelemetry{}
}
func (t *noOpTelemetry) Get() map[string]int64 {
	return map[string]int64{}
}


// CompilationResult enumerates runtime compilation success & failure modes
type CompilationResult int
const (
	Success CompilationResult = iota
	KernelVersionErr
	VerificationError
	OutputDirErr
	OutputFileErr
	NewCompilerErr
	CompilationErr
	ResultReadErr
)

type compilationTelemetry struct {
	enabled bool
	result   CompilationResult
	duration int64
}

func newCompilationTelemetry() *compilationTelemetry {
	return &compilationTelemetry{
		enabled: false,
	}
}

func (t *compilationTelemetry) Get() map[string]int64 {
	stats := make(map[string]int64)
	if t.enabled {
		stats["compilation_enabled"] = 1
		stats["compilation_result"] = int64(t.result)
		stats["compilation_duration"] = t.duration
	} else {
		stats["compilation_enabled"] = 0
	}
	return stats
}

func getCompilationTelemetry(tel Telemetry) *compilationTelemetry  {
	telemetry, ok := tel.(*compilationTelemetry)
	if ok {
		return telemetry
	}
	return newCompilationTelemetry()
}