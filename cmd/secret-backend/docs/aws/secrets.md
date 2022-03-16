# AWS Secrets Manager Backend

> [Datadog Agent Secrets](https://docs.datadoghq.com/agent/guide/secrets-management/?tab=linux) using [AWS Secrets Manager](https://docs.aws.amazon.com/secretsmanager/latest/userguide/intro.html)

## Configuration

### Backend Settings

| Setting | Description |
| --- | --- |
| backend_type | Backend type |
| secret_id | Secret friendly name or Amazon Resource Name |
| aws_session | AWS session configuration |

## Backend Configuration

The backend configuration for AWS Secrets Manager secrets has the following pattern:

```yaml
---
backends:
  {backendId}:
    backend_type: aws.secrets
    aws_session:
      aws_region: {regionName}
      # ... additional session settings
    secret_id: {secretId}

```

**backend_type** must be set to `aws.secrets` and **secret_id** must be set your target AWS Secrets Manager secret friendly name or ARN. The **secret_id** value can be the secret friendly name, e.g. `/DatadogAgent/Production`, or the full ARN format, e.g `arn:aws:secretsmanager:us-east-1:123456789012:secret:/DatadogAgent/Production-FOga1K`. The full ARN format is required when accessing secrets from an a different account where the AWS credential (or sts:AssumeRole credential) is defined.

Cross-account Secrets Manager secrets are supported and tested, but require appropriate permissions on the secret as well as a KMS customer managed key. More details on this configuration is available on the AWS Secrets Manager [documentation](https://docs.aws.amazon.com/secretsmanager/latest/userguide/auth-and-access_examples_cross.html).

The backend secret is referenced in your Datadog Agent configuration files using the **ENC** notation.

```yaml
# /etc/datadog-agent/datadog.yaml

api_key: "ENC[{backendId}:{secretKey}"

```

AWS Secrets Manager can hold multiple secret keys and values. A backend configuration using Secrets Manager will have access to all the secret keys defined on the secret. For example, assuming a secret with a **backend_id** of `MySecretBackend` and a AWS Secrets Manager secret id of `/My/Secret/Backend/Secret`:

```json
{
    "SecretKey1": "SecretValue1",
    "SecretKey2": "SecretValue2",
    "SecretKey3": "SecretValue3"
}
```

```yaml
# /opt/datadog-secret-backend/datadog-secret-backend.yaml
---
backends:
  MySecretBackend:
    backend_type: aws.secrets
    secret_id: /My/Secret/Backend/Secret
    aws_session:
      aws_region: us-east-1
      aws_access_key_id: AKIA*****
      aws_secret_access_key: ************
```

```yaml
# /etc/datadog-agent/datadog.yml
property1: "ENC[MySecretBackend:SecretKey1]"
property2: "ENC[MySecretBackend:SecretKey2]"
property3: "ENC[MySecretBackend:SecretKey3]"
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
api_key: "ENC[agent_secret:api_key]" 
```

**AWS IAM User Access Key with Secrets Manager secret in same AWS account**

```yaml
# /opt/datadog-secret-backend/datadog-secret-backend.yaml
---
backends:
  agent_secret:
    backend_type: aws.secrets
    aws_session:
      aws_region: us-east-1 # set to region of the Secrets Manager secret
      aws_access_key_id: AKIA*****
      aws_secret_access_key: ************
    secret_id: /DatadogAgent/Production
```

**AWS IAM User with sts:AssumeRole**

```yaml
# /opt/datadog-secret-backend/datadog-secret-backend.yaml
---
backends:
  agent_secret:
    backend_type: aws.secrets
    aws_session:
      aws_region: us-east-1 # set to region of the Secrets Manager secret
      aws_access_key_id: AKIA*****
      aws_secret_access_key: ************
      aws_role_arn: arn:aws:iam::123456789012:role/DatadogAgentAssumedRole
      aws_external_id: 3d3e9a6e-f194-4201-a213-0dd1009f6891
    secret_id: arn:aws:secretsmanager:us-east-1:123456789013:secret:/Datadog/Production-FOga1K
```
