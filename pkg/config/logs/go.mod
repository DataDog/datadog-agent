module github.com/DataDog/datadog-agent/pkg/config/logs

go 1.20

replace (
	github.com/DataDog/datadog-agent/pkg/config/model => ../model/
	github.com/DataDog/datadog-agent/pkg/util/log => ../../util/log/
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../util/scrubber/

)

require (
	github.com/DataDog/datadog-agent/pkg/config/model v0.50.0-rc.4
	github.com/DataDog/datadog-agent/pkg/util/log v0.50.0-rc.4
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.50.0-rc.4
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575
	github.com/stretchr/testify v1.8.4
)

require (
	github.com/DataDog/viper v1.12.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.4.7 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/magiconair/properties v1.8.1 // indirect
	github.com/mitchellh/mapstructure v1.1.2 // indirect
	github.com/pelletier/go-toml v1.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/spf13/afero v1.1.2 // indirect
	github.com/spf13/cast v1.3.0 // indirect
	github.com/spf13/jwalterweatherman v1.0.0 // indirect
	github.com/spf13/pflag v1.0.3 // indirect
	golang.org/x/sys v0.5.0 // indirect
	golang.org/x/text v0.3.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
