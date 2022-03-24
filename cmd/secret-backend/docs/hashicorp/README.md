# Hashicorp Secret Backends

## Supported Backends

The `datadog-secret-backend` utility currently supports the following Hashicorp services:

| Backend Type | Hashicorp Service |
| --- | --- |
| [hashicorp.vault](vault.md) | [Hashicorp Vault](https://learn.hashicorp.com/tutorials/vault/static-secrets) |


## Hashicorp ault Session

Hashicorp Vault supports a variety of authentication methods. The ones currently supported by this module are as follows:

1. **App Role Auth**. An App Role id and secret defined on the backed configuration's `vault_session` section within the datadog-secret-backend.yaml file.

2. **User Pass Auth**. A Vault username and password defined on the backed configuration's `vault_session` section within the datadog-secret-backend.yaml file.

3. **LDAP Auth**. An LDAP username and password defined on the backed configuration's `vault_session` section within the datadog-secret-backend.yaml file.

Using environment variables are more complex as they must be configured within the service (daemon) environment configuration or the `dd-agent` user home directory on each Datadog Agent host. Using App Roles and Users (local or LDAP) are simpler configurations which do not require additional Datadog Agent host configuration.

## Vault Session Settings

The following `vault_session` settings are available:

| Setting | Description |
| --- | --- |
| vault_role_id | App Role ID from Vault |
| vault_secret_id | Secret ID for the app role |
| vault_username | Local Vault user |
| vault_password | Password for local vault user |
| vault_ldap_username | LDAP User with Vault access |
| vault_ldap_password | LDAP Password for the LDAP user |

## Example Session Configurations

### Hashicorp Vault Authentication with AppRole

```yaml
---
backends:
  MySecretBackend:
    backend_type: hashicorp.vault
    vault_address: vault_address: http://myvaultaddress.net
    vault_tls_config:
        # ... TLS settings if applicable
    secret_path: /Datadog/Production
    secrets:
      - secret1
      - secret2
    vault_session:
      vault_role_id: 123456-************
      vault_secret_id: abcdef-********
```

### Hashicorp Vault Authentication with UserPass

```yaml
---
backends:
  MySecretBackend:
    backend_type: hashicorp.vault
    vault_address: vault_address: http://myvaultaddress.net
    vault_tls_config:
        # ... TLS settings if applicable
    secret_path: /Datadog/Production
    secrets:
      - secret1
      - secret2
    vault_session:
      vault_username: myuser
      vault_password: mypassword
      
```

### Hashicorp Vault Authentication with LDAP

```yaml
---
backends:
  MySecretBackend:
    backend_type: hashicorp.vault
    vault_address: vault_address: http://myvaultaddress.net
    vault_tls_config:
        # ... TLS settings if applicable
    secret_path: /Datadog/Production
    secrets:
      - secret1
      - secret2
    vault_session:
      vault_ldap_username: myuser
      vault_ldap_password: mypassword
```

Review the [hashicorp.vault](vault.md) backend documentation examples of configurations for Datadog Agent secrets.
