# CHANGELOG - datadog-secret-backend

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
