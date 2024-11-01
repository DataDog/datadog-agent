module github.com/DataDog/datadog-agent/pkg/config/nodetreemodel

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/pkg/config/model => ../../../pkg/config/model
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../pkg/util/scrubber
)

require (
	github.com/DataDog/datadog-agent/pkg/config/model v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/util/log v0.56.0-rc.3
	github.com/DataDog/viper v1.13.5
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826
	github.com/spf13/afero v1.11.0
	github.com/stretchr/testify v1.9.0
	go.uber.org/atomic v1.11.0
	golang.org/x/exp v0.0.0-20241004190924-225e2abe05e6
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.56.0-rc.3 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.4.7 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/magiconair/properties v1.8.1 // indirect
	github.com/mitchellh/mapstructure v1.1.2 // indirect
	github.com/pelletier/go-toml v1.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/spf13/cast v1.3.0 // indirect
	github.com/spf13/jwalterweatherman v1.0.0 // indirect
	github.com/spf13/pflag v1.0.3 // indirect
	golang.org/x/sys v0.26.0 // indirect
	golang.org/x/text v0.19.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
