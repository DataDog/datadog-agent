module github.com/DataDog/datadog-agent/comp/core/tagger/impl-remote

go 1.23.0

replace (
     github.com/DataDog/datadog-agent/comp/core/tagger/subscriber => ../subscriber/
     github.com/DataDog/datadog-agent/comp/core/tagger/telemetry => ../telemetry/
     github.com/DataDog/datadog-agent/comp/core/tagger/generic_store => ../generic_store/



)
