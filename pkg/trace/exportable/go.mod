module github.com/DataDog/datadog-agent/pkg/trace/exportable

go 1.14

replace (
	github.com/DataDog/datadog-agent/pkg/util/log => ../../util/log
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../util/winutil
)

require (
	github.com/DataDog/datadog-agent/pkg/util/log v0.0.0-20201009091026-5e3e70109784
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.0.0-20201009091026-5e3e70109784
	github.com/DataDog/datadog-go v4.0.1+incompatible
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575
	github.com/dgraph-io/ristretto v0.0.3
	github.com/gogo/protobuf v1.3.1
	github.com/philhofer/fwd v1.0.0
	github.com/shirou/gopsutil v2.20.9+incompatible
	github.com/stretchr/testify v1.6.1
	github.com/tinylib/msgp v1.1.2
	github.com/vmihailenco/msgpack/v4 v4.3.12
	golang.org/x/time v0.0.0-20200630173020-3af7569d3a1e
)
