# AWS Secret Backends

## Supported Backends

The `datadog-secret-backend` utility currently supports the following AWS services:

| Backend Type | AWS Service |
| --- | --- |
| [aws.secrets](secrets.md) | [AWS Secrets Manager](https://docs.aws.amazon.com/secretsmanager/latest/userguide/intro.html) |
| [aws.ssm](ssm.md) | [AWS Systems Manager Parameter Store](https://docs.aws.amazon.com/systems-manager/latest/userguide/systems-manager-parameter-store.html) |


## AWS Session

Supported AWS backends can leverage the Default Credential Provider Chain as defined by the AWS SDKs and CLI. As a result, the following order of precedence is used to determine the AWS backend service credential.

1. **Environment Variables**. Note these environment variables would have to be defined as environment variables within the Datadog Agent service configuration on the Datadog Agent host system.

2. **CLI Configuration File**. Currently only the default **CLI Configuration File** of the Datadog Agent user is supported, e.g. `${HOME}/.aws/config` or `%USERPROFILE%\.aws\config`. The Datadog Agent user is typically `dd-agent`.

3. **Instance Profile Credentials**. Container or EC2 hosts with an assigned [IAM Instance Profile](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_use_switch-role-ec2.html)

Using environment variables or session profiles are more complex as they must be configured within the service (daemon) environment configuration or the `dd-agent` user home directory on each Datadog Agent host. Using IAM User Access Keys or an EC2 Instance Profile are simpler configurations which do not require additional Datadog Agent host configuration.

### Instance Profile Instructions

We **highly encourage** to use the instance profile method of retrieving secrets, as AWS handles any environment variables or session profiles for you. 

To use an Instance Profile, create an IAM role in the same account that you are hosting your EC2, ECS, etc. from. Set the "trusted entity type" to the "AWS Service" and choose the relevant service to you (for example, EC2 if you are using an EC2 instance). This IAM role can now be used by an instance of the service that you selected. 

Then, choose a permission policy. Instructions on what permission policy to create are dependent on whether you are using [AWS Secrets](https://github.com/DataDog/datadog-secret-backend/blob/main/docs/aws/secrets.md#iam-permission-needed) or [AWS SSM](https://github.com/DataDog/datadog-secret-backend/blob/main/docs/aws/ssm.md#iam-permission-needed). Finally, set a trust policy--replace ${Service} with the service that you are using:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Principal": {
                "Service": "${Service}.amazonaws.com"
            },
            "Action": "sts:AssumeRole"
        }
    ]
}
```

Now for the instance that you are retrieving secrets for, set the "IAM Role" section to be this role that you have just created. Restart your instance after doing this.

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

When handling single strings, the backend configuration setting `force_string: true` will coerce the secret as a string value. As a result, there will be a single secretId of `_` for the backend and can be accessed in the Datadog Agent yaml as `ENC[{backendId}:_]`.


## Example Session Configurations

### AWS IAM User Access Key for an SSM parameter in us-east-2
```yaml
---
backends:
  my-ssm-secret:
    backend_type: aws.ssm
    aws_session:
      aws_region: us-east-2
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
    secret_id: 'datadog-agent'
```

Review the [aws.ssm](ssm.md) and [aws.secrets](secrets.md) backend documentation examples of configurations for Datadog Agent secrets.
