# AWS Secrets Manager Backend

> [Datadog Agent Secrets](https://docs.datadoghq.com/agent/guide/secrets-management/?tab=linux) using [AWS Secrets Manager](https://docs.aws.amazon.com/secretsmanager/latest/userguide/intro.html)

## Configuration

### IAM Permission Policy (if using an Instance Profile)

Create a similar IAM Permission Policy as the example below to allow resources (EC2, ECS, etc. instances) to access your specified secrets. Please refer to the [AWS Secrets Manager official documentation](https://docs.aws.amazon.com/secretsmanager/) for more details on allowing resources to access secrets. 

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "secretsmanager:GetSecretValue"
      ],
      "Resource": [
        "arn:aws:secretsmanager:${Region}:${Account}:secret:${SecretNameId}"
      ]
    }
  ]
}

```

This is just one step in setting up the Instance Profile. Refer to the [Instance Profile Instructions](https://github.com/DataDog/datadog-secret-backend/blob/main/docs/aws/README.md#instance-profile-instructions) in the AWS README to complete the setup.

### Backend Settings

| Setting | Description |
| --- | --- |
| backend_type | Backend type |
| secret_id | Secret friendly name or Amazon Resource Name |
| aws_session | AWS session configuration |

## Backend Configuration

Ensure that you have followed the instructions specified in the general [aws README](https://github.com/DataDog/datadog-secret-backend/blob/main/docs/aws/README.md) to avoid hardcoding any confidential information in your config file.

The backend configuration for AWS Secrets Manager secrets has the following pattern:

```yaml
# /etc/datadog-agent/datadog.yaml
---
secret_backend_type: aws.secrets
secret_backend_config:
  aws_session:
    aws_region: {regionName}

```

**secret_backend_type** must be set to `aws.secrets`.

Cross-account Secrets Manager secrets are supported and tested, but require appropriate permissions on the secret as well as a KMS customer managed key. More details on this configuration is available on the AWS Secrets Manager [documentation](https://docs.aws.amazon.com/secretsmanager/latest/userguide/auth-and-access_examples_cross.html).

The backend secret is referenced in your Datadog Agent configuration file using the **ENC** notation, taking the form **ENC[secretId;secretKey]**. The **secretId** value can be the secret friendly name, e.g. `/DatadogAgent/Production`, or the full ARN format, e.g `arn:aws:secretsmanager:us-east-1:123456789012:secret:/DatadogAgent/Production-FOga1K`. The full ARN format is required when accessing secrets from an a different account where the AWS credential (or sts:AssumeRole credential) is defined. The **secretKey** is the json key referring to the actual secret that you are trying to pull the value of.

```yaml
# /etc/datadog-agent/datadog.yaml

api_key: ENC[{secretId};{secretKey}]

```

AWS Secrets Manager can hold multiple secret keys and values. A backend configuration using Secrets Manager will have access to all the secret keys defined on the secret. For example, assuming a AWS Secrets Manager secret id of `My-Secret-Backend-Secret`:

```json
{
    "SecretKey1": "SecretValue1",
    "SecretKey2": "SecretValue2",
    "SecretKey3": "SecretValue3"
}
```

```yaml
# /etc/datadog-agent/datadog.yaml
---
secret_backend_type: aws.secrets
secret_backend_config:
  aws_session:
    aws_region: us-east-1
```

```yaml
# /etc/datadog-agent/datadog.yaml
property1: ENC[My-Secret-Backend-Secret;SecretKey1]
property2: ENC[My-Secret-Backend-Secret;SecretKey2]
property3: ENC[My-Secret-Backend-Secret;SecretKey3]
```

Multiple secret backends, of the same or different types, can be defined in your `datadog-secret-backend` yaml configuration. As a result, you can leverage multiple supported backends (file.yaml, file.json, aws.ssm, and aws.secrets) in your Datadog Agent configuration.

## Configuration Examples

In the following examples, assume the AWS Secrets Manager secret friendly name (id) is `/DatadogAgent/Production` with a secret value containing the Datadog Agent api_key:

```json
{
    "api_key": "••••••••••••0f83"
}
```

Each of the following examples will access the secret from the Datadog Agent configuration yaml file(s) as such:

```yaml
# /etc/datadog-agent/datadog.yaml

#########################
## Basic Configuration ##
#########################

## @param api_key - string - required
## @env DD_API_KEY - string - required
## The Datadog API key to associate your Agent's data with your organization.
## Create a new API key here: https://app.datadoghq.com/account/settings
#
api_key: ENC[/DatadogAgent/Production;api_key] 
```

**AWS IAM User Access Key with Secrets Manager secret in same AWS account**

```yaml
# /etc/datadog-agent/datadog.yaml
---
secret_backend_type: aws.secrets
secret_backend_config:
  aws_session:
    aws_region: us-east-1
```
