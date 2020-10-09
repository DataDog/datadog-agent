module github.com/DataDog/datadog-agent/pkg/trace/exportable

go 1.14

replace (
	github.com/DataDog/datadog-agent/pkg/util/log => ../../util/log
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../util/winutil
)

require (
	github.com/DataDog/datadog-agent/pkg/util/log v0.0.0-20201009092105-58e18918b2db
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.0.0-20201009092105-58e18918b2db
	github.com/DataDog/datadog-go v3.5.0+incompatible
	github.com/StackExchange/wmi v0.0.0-20190523213315-cbe66965904d // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575
	github.com/dgraph-io/ristretto v0.0.3
	github.com/go-ole/go-ole v1.2.4 // indirect
	github.com/gogo/protobuf v1.3.1
	github.com/philhofer/fwd v1.0.0
	github.com/shirou/gopsutil v2.20.9+incompatible
	github.com/stretchr/testify v1.6.1
	github.com/tinylib/msgp v1.1.2
	github.com/vmihailenco/msgpack/v4 v4.3.12
	golang.org/x/sys v0.0.0-20190215142949-d0b11bdaac8a
	golang.org/x/time v0.0.0-20200630173020-3af7569d3a1e
)
