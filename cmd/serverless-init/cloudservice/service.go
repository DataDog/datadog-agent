package cloudservice

// CloudService implements getting tags from each Cloud Provider.
type CloudService interface {
	// GetTags returns a map of tags for a given cloud service. These tags are then attached to
	// the logs, traces, and metrics.
	GetTags() map[string]string

	// GetOrigin returns the value that will be used for the `origin` attribute for
	// all logs, traces, and metrics.
	GetOrigin() string

	// GetPrefix returns the prefix that we're prefixing all
	// metrics with. For example, for cloudrun, we're using
	// gcp.run.{metric_name}. In this example, `gcp.run` is the
	// prefix.
	GetPrefix() string
}

func GetCloudServiceType() CloudService {
	if isContainerAppService() {
		return &ContainerApp{}
	}

	// By default, we're in CloudRun
	return &CloudRun{}
}
