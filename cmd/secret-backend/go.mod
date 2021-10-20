module github.com/rapdev-io/datadog-secret-backend

go 1.17

replace (
	github.com/rapdev-io/datadog-secret-backend/backend => ./backend
	github.com/rapdev-io/datadog-secret-backend/backend/aws => ./backend/aws
	github.com/rapdev-io/datadog-secret-backend/backend/aws/secretsmanager => ./backend/aws/secretsmanager
)

require (
	github.com/aws/aws-sdk-go-v2 v1.9.0
	github.com/aws/aws-sdk-go-v2/config v1.7.0
	github.com/aws/aws-sdk-go-v2/credentials v1.4.0
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.6.0
	github.com/aws/aws-sdk-go-v2/service/ssm v1.10.0
	github.com/aws/aws-sdk-go-v2/service/sts v1.7.0
	github.com/mitchellh/mapstructure v1.4.1
	github.com/rs/zerolog v1.25.0
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.5.0 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.2.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.3.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.4.0 // indirect
	github.com/aws/smithy-go v1.8.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
)
