# Akeyless Secret Backends

## Supported Backends

The `datadog-secret-backend` utility currently supports the following Hashicorp services:

| Backend Type | Akeyless Service                                                              |
| --- |-------------------------------------------------------------------------------|
| [hashicorp.vault](secrets) | [Hashicorp Vault](https://learn.hashicorp.com/tutorials/vault/static-secrets) |


## Akeyless Session

Akeyless requires authentication to access secrets. This module utilizes Akeyless access IDs and access keys for authentication. You'll configure these credentials within the Datadog Secret Backend configuration file.


## Akeyless Session Settings

The following sections detail the configuration options for the Akeyless backend:

| Setting | Description                                                 |
| --- |-------------------------------------------------------------|
| backend_type | Must be set to `akeyless`                                     |
| akeyless_url | URL of your Akeyless instance (e.g., https://myakeyless.io) |
|akeyless_session | 	Configuration for the Akeyless session                     |

The `akeyless_session` section defines the credentials used to authenticate with Akeyless.

| Setting | Description |
| --- | --- |
| akeyless_access_id | Akeyless Access ID |
| akeyless_access_key | Akeyless Access Key |


## Example Session Configurations

The backend configuration for Akeyless follows this pattern:

```yaml
---
backends:
  akeyless:
    backend_type: 'akeyless'
    akeyless_url: 'https://api.akeyless.io'
    akeyless_session:
      akeyless_access_id: 'abcdef123456**********'
      akeyless_access_key: 'abcdef123456**********'
```

Review the [akeyless.secrets](secrets.md) backend documentation examples of configurations for Datadog Agent secrets.
