# AWS Secret Backends

## Supported Backends

The `datadog-secret-backend` utility currently supports the following AWS services:

| Backend Type | AWS Service |
| --- | --- |
| [aws.ssm](ssm.md) | [AWS Systems Manager Parameter Store](https://docs.aws.amazon.com/systems-manager/latest/userguide/systems-manager-parameter-store.html) |
| [aws.secrets](secrets.md) | [AWS Secrets Manager](https://docs.aws.amazon.com/secretsmanager/latest/userguide/intro.html) |

## AWS Sessions

Supported AWS backends can leverage the Default Credential Provider Chain as defined by the AWS SDKs and CLI. As a result, the following order of precedence is used to determine the AWS backend service credential.

1. **IAM User Access Key**. The access key id and secret value are defined in each backed configuration within the datadog-secret-backend.yaml file.

2. **Environment Variables**. Note these environment variables would have to be defined as environment variables within service configuration for the datadog-agent on the host system.

3. **CLI Credentials File**. Currently only the default **CLI Credential File** of the Datadog Agent user is supported, e.g. `${HOME}/.aws/credentials` or `%USERPROFILE%\.aws\credentials`. The Datadog Agent user is typically `dd-agent`.

4. **CLI Configuration File**. Currently only the default **CLI Configuration File** of the Datadog Agent user is supported, e.g. `${HOME}/.aws/config` or `%USERPROFILE%\.aws\config`. The Datadog Agent user is typically `dd-agent`.

5. **Instance Profile Credentials**. Container or EC2 hosts with an assigned [IAM Instance Profile](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_use_switch-role-ec2.html)

## Common AWS Session Settings

The following `aws_session` settings are available on all supported AWS Service backends:

| Backend Type | Setting | Example Value |
| --- | --- | --- |
| | | 