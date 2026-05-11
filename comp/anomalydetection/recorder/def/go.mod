module github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def

go 1.25.0

require github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def v0.0.0-00010101000000-000000000000

// This section was automatically added by 'dda inv modules.add-all-replace' command, do not edit manually

replace (
	github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def => ../../../../comp/anomalydetection/observer/def
)
