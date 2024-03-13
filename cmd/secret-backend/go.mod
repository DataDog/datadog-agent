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
	github.com/hashicorp/vault/api v1.4.1
	github.com/hashicorp/vault/api/auth/approle v0.1.1
	github.com/hashicorp/vault/api/auth/ldap v0.1.0
	github.com/hashicorp/vault/api/auth/userpass v0.1.0
	github.com/mitchellh/mapstructure v1.4.2
	github.com/rs/zerolog v1.25.0
	github.com/sirupsen/logrus v1.8.1
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/Azure/azure-sdk-for-go v62.1.0+incompatible
	github.com/Azure/go-autorest/autorest v0.11.17
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.8
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.5.0 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.2.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.3.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.4.0 // indirect
	github.com/aws/smithy-go v1.8.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c // indirect
)

require (
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest/adal v0.9.11 // indirect
	github.com/Azure/go-autorest/autorest/azure/cli v0.4.2 // indirect
	github.com/Azure/go-autorest/autorest/date v0.3.0 // indirect
	github.com/Azure/go-autorest/autorest/to v0.4.0 // indirect
	github.com/Azure/go-autorest/autorest/validation v0.3.1 // indirect
	github.com/Azure/go-autorest/logger v0.2.0 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/akeylesslabs/akeyless-go/v2 v2.20.3 // indirect
	github.com/armon/go-metrics v0.3.9 // indirect
	github.com/armon/go-radix v1.0.0 // indirect
	github.com/cenkalti/backoff/v3 v3.0.0 // indirect
	github.com/dimchansky/utfbom v1.1.1 // indirect
	github.com/fatih/color v1.7.0 // indirect
	github.com/form3tech-oss/jwt-go v3.2.2+incompatible // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v0.16.2 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-plugin v1.4.3 // indirect
	github.com/hashicorp/go-retryablehttp v0.6.6 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-secure-stdlib/mlock v0.1.1 // indirect
	github.com/hashicorp/go-secure-stdlib/parseutil v0.1.1 // indirect
	github.com/hashicorp/go-secure-stdlib/strutil v0.1.1 // indirect
	github.com/hashicorp/go-sockaddr v1.0.2 // indirect
	github.com/hashicorp/go-uuid v1.0.2 // indirect
	github.com/hashicorp/go-version v1.2.0 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hashicorp/vault/sdk v0.4.1 // indirect
	github.com/hashicorp/yamux v0.0.0-20180604194846-3520598351bb // indirect
	github.com/mattn/go-colorable v0.1.6 // indirect
	github.com/mattn/go-isatty v0.0.12 // indirect
	github.com/mitchellh/copystructure v1.0.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/go-testing-interface v1.0.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.0 // indirect
	github.com/oklog/run v1.0.0 // indirect
	github.com/pierrec/lz4 v2.5.2+incompatible // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	golang.org/x/crypto v0.0.0-20210711020723-a769d52b0f97 // indirect
	golang.org/x/net v0.0.0-20210405180319-a5a99cb37ef4 // indirect
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d // indirect
	golang.org/x/text v0.3.3 // indirect
	golang.org/x/time v0.0.0-20200416051211-89c76fbcd5d1 // indirect
	google.golang.org/appengine v1.4.0 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
	google.golang.org/grpc v1.41.0 // indirect
	google.golang.org/protobuf v1.26.0 // indirect
	gopkg.in/square/go-jose.v2 v2.5.1 // indirect
)
