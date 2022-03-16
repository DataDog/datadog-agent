# AWS Secret Backends

## Supported Backends

The `datadog-secret-backend` utility currently supports the following AWS services:

| Backend Type | AWS Service |
| --- | --- |
| [aws.secrets](secrets.md) | [AWS Secrets Manager](https://docs.aws.amazon.com/secretsmanager/latest/userguide/intro.html) |
| [aws.ssm](ssm.md) | [AWS Systems Manager Parameter Store](https://docs.aws.amazon.com/systems-manager/latest/userguide/systems-manager-parameter-store.html) |


## AWS Session

Supported AWS backends can leverage the Default Credential Provider Chain as defined by the AWS SDKs and CLI. As a result, the following order of precedence is used to determine the AWS backend service credential.

1. **IAM User Access Key**. An Access Key id and secret defined on the backed configuration's `aws_session` section within the datadog-secret-backend.yaml file.

2. **Environment Variables**. Note these environment variables would have to be defined as environment variables within the Datadog Agent service configuration on the Datadog Agent host system.

3. **CLI Credentials File**. Currently only the default **CLI Credential File** of the Datadog Agent user is supported, e.g. `${HOME}/.aws/credentials` or `%USERPROFILE%\.aws\credentials`. The Datadog Agent user is typically `dd-agent`.

4. **CLI Configuration File**. Currently only the default **CLI Configuration File** of the Datadog Agent user is supported, e.g. `${HOME}/.aws/config` or `%USERPROFILE%\.aws\config`. The Datadog Agent user is typically `dd-agent`.

5. **Instance Profile Credentials**. Container or EC2 hosts with an assigned [IAM Instance Profile](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_use_switch-role-ec2.html)

Using environment variables or session profiles are more complex as they must be configured within the service (daemon) environment configuration or the `dd-agent` user home directory on each Datadog Agent host. Using IAM User Access Keys or an EC2 Instance Profile are simpler configurations which do not require additional Datadog Agent host configuration.

## AWS Session Settings

The following `aws_session` settings are available on all supported AWS Service backends:

| Setting | Description |
| --- | --- |
| aws_region | AWS Region |
| aws_profile | AWS Session Profile |
| aws_role_arn | AWS sts:AssumeRole ARN |
| aws_external_id | AWS sts:AssumeRole ExternalId |
| aws_access_key_id | AWS IAM User Access Key ID |
| aws_secret_access_key | AWS IAM User Access Key Secret |

In most cases, you'll need to specify `aws_region` to correspond to the region hosting the target Parameter Store (aws.ssm) or Secrets Manager (aws.secrets) secret.

## Example Session Configurations

### AWS IAM User Access Key for an SSM parameter in us-east-2
```yaml
---
backends:
  my-ssm-secret:
    backend_type: aws.ssm
    aws_session:
      aws_region: us-east-2
      aws_access_key_id: AKIA*****
      aws_secret_access_key: ************
    parameters: 
      - /My/Secret/Path/To/Secret
```

### AWS Credential Provider Profile for a Secrets Manager secret in us-east-1
```yaml
---
backends:
  my-ssm-secret:
    backend_type: aws.secrets
    aws_session:
      aws_region: us-east-1
      aws_profile: datadog-agent-profile # defined in .aws/config or .aws/credentials
    secret_id: 'datadog-agent'
```

### AWS with cross-account assume role with an externalId trust condition
```yaml
---
backends:
  my-ssm-secret:
    backend_type: aws.secrets
    aws_session:
      aws_region: us-east-1
      aws_access_key_id: AKIA*****
      aws_secret_access_key: ************
      aws_role_arn: arn:aws:iam::123456789012:role/DatadogSecretsBackend
      aws_external_id: unique-external-id-string-value
    secret_id: 'datadog-agent'
```

* `aws_role_arn` works with all AWS Default Credential Chain options. However, the credential from the Default Credential Chain must have enough permissions to assume the target role and include `aws_external_id` if the assume role trust policy requires it.

Review the [aws.ssm](ssm.md) and [aws.secrets](secrets.md) backend documentation examples of configurations for Datadog Agent secrets.
