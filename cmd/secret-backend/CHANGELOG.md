# CHANGELOG - datadog-secret-backend

## 1.4.1 / 2025-11-25

* Rework GCP delimiter parsing
* Bump github.com/aws/aws-sdk-go-v2/service/ssm from 1.67.2 to 1.67.3
* Bump github.com/aws/aws-sdk-go-v2/credentials from 1.18.24 to 1.19.0
* Bump github.com/aws/aws-sdk-go-v2/service/secretsmanager from 1.39.13 to 1.40.1
* Bump github.com/aws/aws-sdk-go-v2/config from 1.31.20 to 1.32.1
* Bump github.com/hashicorp/vault from 1.21.0 to 1.21.1

## 1.4.0 / 2025-11-18

* Add support for GCP as a backend type
* Add secret-backend timeout based on datadog-agents `secret_backend_timeout` config
* Bump github.com/Azure/azure-sdk-for-go/sdk/azidentity from 1.13.0 to 1.13.1
* Bump github.com/aws/aws-sdk-go-v2/config from 1.31.19 to 1.31.20
* Bump github.com/aws/aws-sdk-go-v2/service/secretsmanager from 1.39.11 to 1.39.13
* Bump github.com/aws/aws-sdk-go-v2/service/sts from 1.40.1 to 1.40.2
* Bump github.com/aws/aws-sdk-go-v2/service/ssm from 1.67.0 to 1.67.2

## 1.3.4 / 2025-11-13

* Allow AWS Secrets Manager secret values to be non-strings (which will be stringified during retrieval)
* Bump aws-sdk-go-v2, multiple packages (config, ssm, credentials)

## 1.3.3 / 2025-11-07

* Fixing k8s login issue by handling authentication manually 
* Bump github.com/aws/aws-sdk-go-v2/config to 1.31.17
* Bump github.com/aws/aws-sdk-go-v2/service/secretsmanager to 1.39.11
* Bump github.com/aws/aws-sdk-go-v2/service/sts to 1.39.1
* Bump github.com/aws/aws-sdk-go-v2/service/ssm to 1.66.4
* Bump github.com/aws/aws-sdk-go-v2/credentials to 1.18.21
* Bump github.com/Azure/azure-sdk-for-go/sdk/azcore to 1.20.0 

## 1.3.2 / 2025-10-31

* Bump go version to `1.25.3`

## 1.3.1 / 2025-10-27

* Allow for the passing in of the VAULT_ADDR env var
* Use go 1.25.1 to build release.
* Bump github.com/hashicorp/vault fropm 1.20.4 to v1.21.0
* Bump github.com/aws/aws-sdk-go-v2 from 1.39.3 to 1.39.4
* Bump github.com/aws/aws-sdk-go-v2/config from 1.31.8 to 1.31.15
* Bump github.com/aws/aws-sdk-go-v2/service/secretsmanager 1.39.4 to 1.39.9
* Bump github.com/aws/aws-sdk-go-v2/service/sts from 1.38.8 to 1.38.9
* Bump github.com/aws/aws-sdk-go-v2/service/ssm from 1.64.4 to 1.66.2
* Bump github.com/aws/aws-sdk-go-v2/credentials from 1.18.17 to 1.18.18
* Bump github.com/Azure/azure-sdk-for-go/sdk/azidentity from 1.12.0 to 1.13.0
* Bump github.com/hashicorp/vault/api/auth/userpass from 0.10.0 to 0.11.0
* Bump github.com/hashicorp/vault/api from 1.21.0 to 1.22.0
* Bump github.com/hashicorp/vault/api/auth/ldap from 0.10.0 to 0.11.0
* Bump github.com/hashicorp/vault/api/auth/approle from 0.10.0 to 0.11.0

## 1.3.0 / 2025-09-23

* Bump aws-sdk-go-v2, multiple packages (config, ssm, secretsmanager)
* Update hashicorp/vault/api to 1.21.0
* Bumped azure-sdk-for-go/sdk/azidentity to 1.12.0
* Fixing the release job by not disabling the GC write-barrier
* Enable k8s authentication with Hashicorp Vault

## 1.2.0 / 2025-09-04

* Bump aws-sdk-go-v2, multiple packages (config, ssm, secretsmanager, rts)
* Bump Azure/go-autorest and azcore
* Either get clientID from stdin, or env var, otherwise try default identity
* Update Hashicorp Vault to 1.20.3
* Removing unused code and simplify init logic
* Reduce the binary size for the release

## 1.1.1 / 2025-08-14

* Use go 1.24.6 to build release.

## 1.1.0 / 2025-08-05

* Removing the `zerolog` library.
* Removing the `logrus` dependency.
* Adding support for version 2 of the Hashicorp Secrets Engine.

## 1.0.1 / 2025-07-16

* Replacing the dependency on `hashicorp/vault/api/auth/aws` with the forked `DataDog/vault/api/auth/aws` library.

## 1.0.0 / 2025-07-10

* Switched Azure backend to `azsecrets`, removed `go-autorest`.
* Enabled Azure managed identity for secret retrieval.
* Accepting config input via stdin, not separate files.
* Azure secrets can now be flat strings or JSON.
* Removing `secret_id` from AWS Secrets config.
* Removing `parameters_path` from AWS SSM config.
* Removing `secret_path` from Hashicorp config.
* Centralizing secret retrieval in GetSecretOutput.
* Updating Azure Key Vault docs with semicolon syntax.
* Fixing Azure test bug and formatting issues.
* Updated `release.yaml` job to automatically bump `appVersion`.

## 0.2.5 / 2025-06-27

* Bump go version to `1.24.4`

## 0.2.4 / 2025-06-12

* Bump cloudflare/circl to `v1.6.1`
* Bump requests to `v2.32.4`
* Fixing link to AWS docs
* Bump appVersion to 0.2.4

## 0.2.3 / 2025-05-20

* Bump go-git/go-git/v5 to `v5.13.0`
* Bump golang-jwt/jwt/v4 to `v4.5.2`
* Bump golang-jwt/jwt/v5 to `v5.2.2`
* Bump hashicorp/vault to `v1.19.3`
* Bump go-jose/go-jose/v3 to `v3.0.4`
* Bump go-jose/go-jose/v4 to `v4.0.5`
* Limiting workflow permissions to `contents: read` and `pull-requests: write`

## 0.2.2 / 2025-05-19

* Bump hashicorp/vault/api from `v1.15.0` to `v1.16.0`
* Bump golang.org/x/net from `v0.34.0` to `v0.40.0`

## 0.2.1 / 2025-05-07

* Release latest version of the datadog-secret-backend without debug and DWARF symbol.

## 0.2.0 / 2025-04-28

* Build the artifact without debug and DWARF symbol to produce smaller binaries (`-ldflags="-s -w"` is used).

## 0.1.14 / 2025-03-24

* [Fix] Work around Azure issue 39434 & support escaped json strings
* [Documentation] Add permission needed to use aws parameter store
* [CI] Add generate licenses tasks and run them on each PR
* [CI] Running copyrights linter on each PR

## 0.1.13 / 2024-11-19

* Repo ownership transitioned from RapDev to Datadog.
* [Fix] Clean up version flag handling.
* [CI] Adding golangci-lint to the CI and fixing all warnings from the linters.
* [Documentation] Updating contribution guidelines and adding Issue and PR GH templates.

## 0.1.12 / 2024-09-13

* [Added] CI now produces ARM64 artefacts.

## 0.1.11 / 2024-03-20

* [Added] new backend configuration for Akeyless Secrets.

## 0.1.10 / 2022-08-17

* [Added] support for simple string value secrets in AWS Secrets Manager and Azure Key Vault.

## 0.1.7 / 2021-10-20

* [Added] zerolog logger, replacing logrus.
* [Fixed] documentation, adding usage of aws.ssm and aws.secrets backends.
