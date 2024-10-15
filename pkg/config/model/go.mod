module github.com/DataDog/datadog-agent/pkg/config/model

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/pkg/config/mock => ../../../pkg/config/mock/
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel => ../../../pkg/config/nodetreemodel/
	github.com/DataDog/datadog-agent/pkg/config/setup => ../../../pkg/config/setup/
	github.com/DataDog/datadog-agent/pkg/config/structure => ../../../pkg/config/structure/
	github.com/DataDog/datadog-agent/pkg/config/teeconfig => ../../../pkg/config/teeconfig
	github.com/DataDog/datadog-agent/pkg/util/log => ../../util/log/
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../util/scrubber/
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../util/system/socket/
)

require (
	github.com/DataDog/datadog-agent/pkg/config/structure v0.60.0-devel
	github.com/DataDog/datadog-agent/pkg/util/log v0.56.0-rc.3
	github.com/DataDog/viper v1.13.5
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826
	github.com/spf13/afero v1.11.0
	github.com/stretchr/testify v1.9.0
	golang.org/x/exp v0.0.0-20241004190924-225e2abe05e6
)

require (
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.56.0-rc.3 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/magiconair/properties v1.8.1 // indirect
	github.com/mitchellh/mapstructure v1.1.2 // indirect
	github.com/pelletier/go-toml v1.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/spf13/cast v1.5.1 // indirect
	github.com/spf13/jwalterweatherman v1.0.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/sys v0.26.0 // indirect
	golang.org/x/text v0.19.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
