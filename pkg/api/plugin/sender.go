package plugin

// Sender allows sending metrics from checks/a check
type Sender interface {
	Gauge(metric string, value float64, hostname string, tags []string)
	Rate(metric string, value float64, hostname string, tags []string)
	Count(metric string, value float64, hostname string, tags []string)
	MonotonicCount(metric string, value float64, hostname string, tags []string)
	MonotonicCountWithFlushFirstValue(metric string, value float64, hostname string, tags []string, flushFirstValue bool)
	Counter(metric string, value float64, hostname string, tags []string)
	Histogram(metric string, value float64, hostname string, tags []string)
	Historate(metric string, value float64, hostname string, tags []string)
	HistogramBucket(metric string, value int64, lowerBound, upperBound float64, monotonic bool, hostname string, tags []string)
	GetMetricStats() map[string]int64
	DisableDefaultHostname(disable bool)
	SetCheckCustomTags(tags []string)
	SetCheckService(service string)
	FinalizeCheckServiceTag()

	//	ServiceCheck(checkName string, status metrics.ServiceCheckStatus, hostname string, tags []string, message string)
	//	Event(e metrics.Event)
	//	OrchestratorMetadata(msgs []serializer.ProcessMessageBody, clusterID, payloadType string)

	Commit()
}
