module github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip

go 1.21.0

replace github.com/DataDog/datadog-agent/comp/trace/compression/def => ../../../../comp/trace/compression/def/

require github.com/DataDog/datadog-agent/comp/trace/compression/def v0.56.0-rc.2
