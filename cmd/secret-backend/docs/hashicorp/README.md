# Hashicorp Secret Backends

## Supported Backends

The `datadog-secret-backend` utility currently supports the following Hashicorp services:

| Backend Type | Hashicorp Service |
| --- | --- |
| [hashicorp.vault](vault.md) | [Hashicorp Vault](https://learn.hashicorp.com/tutorials/vault/static-secrets) |


## Hashicorp auth Session

Hashicorp Vault supports a variety of authentication methods. The ones currently supported by this module are as follows:

1. **User Pass Auth**. A Vault username and password defined on the backed configuration's `vault_session` section within the datadog-secret-backend.yaml file.

2. **AWS Instance Profile** If your machine has an AWS IAM role attached to it with the correct permissions, you don't need to define any secret credentials/passwords in your config. Refer to the [AWS Instance Profile section](https://github.com/DataDog/datadog-secret-backend/blob/main/docs/aws/README.md#instance-profile-instructions) and the [official Hashicorp AWS auth method instructions](https://developer.hashicorp.com/vault/docs/auth/aws#aws-auth-method) for more information.

Using environment variables are more complex as they must be configured within the service (daemon) environment configuration or the `dd-agent` user home directory on each Datadog Agent host. Using App Roles and Users (local or LDAP) are simpler configurations which do not require additional Datadog Agent host configuration.

## General Instructions to set up Hashicorp Vault
1. Run your Hashicorp Vault. For more information on how to do this, please look at the [official Hashicorp Vault documentation](https://www.hashicorp.com/en/products/vault). 
2. When running the vault, you should have received the variables `VAULT_ADDR` and `VAULT_TOKEN`. Export them as environment variables.
3. To store your secrets in a certain path, run `vault secrets enable -path=<your path> kv`
4. To add your key, run `vault kv put <your path> apikey=your_real_datadog_api_key`. You can conversely run `vault kv get ...` to get said key.
5. Now you need to write a policy to give permission to pull secrets from your vault. Create a *.hcl file, and include the following permission:
```
path "<your path>" {
  capabilities = ["read"]
}
```
Now run `vault policy write <policy-name> <path to *.hcl file>`
6. Now you need to choose the method of authenticating to your vault. If using the AWS Instance Profile method, run `vault auth enable aws`. 

### AWS Instance Profile Instructions

We HIGHLY recommend that you authenticate using this method if you are running your Hashicorp Vault from an AWS-connected machine. Please first do the setup described in the [general AWS Instruction Profile Instructions](https://github.com/DataDog/datadog-secret-backend/blob/main/docs/aws/README.md#instance-profile-instructions). After following those instructions, you should have attached a policy to your IAM role. To this policy, you will need to add the `sts:GetCallerPolicy` permission as well:
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "sts:GetCallerIdentity",
      "Resource": "*"
    }
  ]
}
```
After following the General Instructions, you will aslo need to write an authentication-specific vault policy. Run:
```
vault write auth/aws/role/rahul_role \
  auth_type=iam \
  bound_iam_principal_arn=arn:aws:iam::<AWS Account ID>:role/<Name of AWS IAM Role> \
  policies=<name of *.hcl file policy> \
  max_ttl=768h
```

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
| vault_auth_type | The backend service if using an instance profile |
| vault_aws_role | The name of the IAM user if vault_auth_type is 'aws' |
| aws_region | The AWS region of the machine if vault_auth_type is 'aws' |

## Example Session Configurations

### Hashicorp Vault Authentication with AWS Instance Profile

```yaml
# /etc/datadog-agent/datadog.yaml
---
secret_backend_type: hashicorp.vault
secret_backend_config:
  vault_address: vault_address: http://myvaultaddress.net
  secret_path: /Datadog/Production
  vault_session:
    vault_auth_type: aws
    vault_aws_role: Name-of-IAM-role-attached-to-machine
    aws_region: us-east-1
```

Review the [hashicorp.vault](vault.md) backend documentation examples of configurations for Datadog Agent secrets.