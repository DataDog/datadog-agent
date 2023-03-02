package metrics

// MetricSource represents how this metric made it into the Agent
type MetricSource int

// Enumeration of the existing API metric types
const (
	MetricSourceUnknown MetricSource = iota
	// TODO this conflates source and service
	// and also gives no 'string' field for metric name
	MetricSourceDogstatsd
	MetricSourceActiveDirectory
	MetricSourceSystemCore
	MetricSourceActiveMqXml
	MetricSourceDocker
	MetricSourceOpenMetrics
	MetricSourceNtp
)

func CheckNameToMetricSource(checkName string) MetricSource {
	switch checkName {
	case "active_directory":
		return MetricSourceActiveDirectory
	case "system", "io", "load", "cpu", "memory", "uptime", "file_handle":
		return MetricSourceSystemCore
	case "openmetrics":
		return MetricSourceOpenMetrics
	case "docker":
		return MetricSourceDocker
	case "ntp":
		return MetricSourceNtp
	}

	return MetricSourceUnknown
}

// String returns a string representation of APIMetricType
func (ms MetricSource) String() string {
	switch ms {
	case MetricSourceDogstatsd:
		return "dogstatsd"
	case MetricSourceSystemCore:
		return "systemcore"
	case MetricSourceOpenMetrics:
		return "openmetrics"
	default:
		return "<unknown>"
	}
}

func (ms MetricSource) OriginCategory() int32 {
	// Constants from `origin.proto`
	switch ms {
	case MetricSourceDogstatsd:
		return 10
	default:
		// integration
		return 11
	}
}

func (ms MetricSource) OriginService() int32 {
	// Constants from `origin.proto`
	switch ms {
	case MetricSourceDogstatsd:
		return 0 // no service
	case MetricSourceActiveDirectory:
		return 10
	case MetricSourceActiveMqXml:
		return 11
	case MetricSourceSystemCore:
		return 155
	default:
		return -1
	}
}
