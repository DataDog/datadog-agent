module github.com/DataDog/datadog-agent/pkg/config/nodetreemodel

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/pkg/config/model => ../../../pkg/config/model
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../pkg/util/scrubber
)

// Internal deps fix version
replace github.com/spf13/cast => github.com/DataDog/cast v1.8.0

require (
	github.com/DataDog/datadog-agent/pkg/config/model v0.63.0-rc.6
	github.com/DataDog/datadog-agent/pkg/util/log v0.63.0-rc.6
	github.com/DataDog/viper v1.14.0
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826
	github.com/spf13/cast v1.7.1
	github.com/stretchr/testify v1.10.0
	go.uber.org/atomic v1.11.0
	golang.org/x/exp v0.0.0-20250106191152-7588d65b2ba8
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.63.0-rc.6 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.63.0-rc.6 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-5 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mitchellh/mapstructure v1.5.1-0.20231216201459-8508981c8b6c // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/spf13/afero v1.11.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/DataDog/datadog-agent/pkg/version => ../../version
