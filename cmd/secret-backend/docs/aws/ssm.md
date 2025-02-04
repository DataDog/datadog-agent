# AWS Systems Manager Parameter Store Backend

> [Datadog Agent Secrets](https://docs.datadoghq.com/agent/guide/secrets-management/?tab=linux) using [AWS Systems Manager Parameter Store](https://docs.aws.amazon.com/systems-manager/latest/userguide/systems-manager-parameter-store.html)

## Configuration

### IAM Permission needed

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "ssm:GetParameter",
      "Resource": [
        "arn:aws:ssm:${Region}:${Account}:parameter/${ParameterNameWithoutLeadingSlash}"
      ]
    }
  ]
}

```

### Backend Settings

| Setting | Description |
| --- | --- |
| backend_type | Backend type |
| parameter_path| SSM parameters prefix, recursive |
| parameters | List of individual SSM parameters |

## Backend Configuration

The backend configuration for AWS SSM Parameter Store secrets has the following pattern:

```yaml
---
backends:
  {backendId}:
    backend_type: aws.ssm
    aws_session:
      aws_region: {regionName}
      # ... additional session settings
    parameter_path: /Path/To/Parameters
    parameters:
      - /Path/To/Parameters/Parameter1
      - /Path/To/Parameters/Parameter2
      - /Path/To/Parameters/Parameter3
```

**backend_type** must be set to `aws.ssm` and either or both of **parameter_path** and **parameters** must be provided in each backend configuration.

The backend secret is referenced in your Datadog Agent configuration files using the **ENC** notation.

```yaml
# /etc/datadog-agent/datadog.yaml

api_key: "ENC[{backendId}:{parameter_full_path}"

```

AWS System Manager Parameter store supports a heirachical model. Parameters can be specified individually using **parameters**, or recursively fetched using a matching
 prefix path with **parameter_path**. For example, assuming a secret with a **backend_id** of `MySecretBackend` and the AWS System Manager Parameter Store paths

```sh
/DatadogAgent/Production/ParameterKey1 = ParameterStringValue1
/DatadogAgent/Production/ParameterKey2 = ParameterStringValue2
/DatadogAgent/Production/ParameterKey3 = ParameterStringValue3
```

The parameters can be fetched using **parameter_path**:

```yaml
# /opt/datadog-secret-backend/datadog-secret-backend.yaml
---
backends:
  MySecretBackend:
    backend_type: aws.secrets
    parameter_path: /DatadogAgent/Production
    aws_session:
      aws_region: us-east-1
      aws_access_key_id: AKIA*****
      aws_secret_access_key: ************
```

or fetched using **parameters**:

```yaml
# /opt/datadog-secret-backend/datadog-secret-backend.yaml
---
backends:
  MySecretBackend:
    backend_type: aws.secrets
    parameters: 
      - /DatadogAgent/Production/ParameterKey1
      - /DatadogAgent/Production/ParameterKey2
      - /DatadogAgent/Production/ParameterKey3
    aws_session:
      aws_region: us-east-1
      aws_access_key_id: AKIA*****
      aws_secret_access_key: ************
```

and finally accessed in the Datadog Agent:

```yaml
# /etc/datadog-agent/datadog.yml
property1: "ENC[MySecretBackend:/DatadogAgent/Production/ParameterKey1]"
property2: "ENC[MySecretBackend:/DatadogAgent/Production/ParameterKey2]"
property3: "ENC[MySecretBackend:/DatadogAgent/Production/ParameterKey3]"
```

Currently, `StringList` parameter store values will be retained as a comma-separated list. `SecureString` will be properly decrypted automatically, assuming the `aws_session` credentials have appropriate rights to the KMS key used to encrypt the `SecureString` value.

Multiple secret backends, of the same or different types, can be defined in your `datadog-secret-backend` yaml configuration. As a result, you can leverage multiple supported backends (file.yaml, file.json, aws.ssm, and aws.secrets) in your Datadog Agent configuration.

## Configuration Examples

In the following examples, assume the AWS Systems Manager Parameter Store secret path prefix is `/DatadogAgent/Production` with a parameter key of `api_key`:

```sh
/DatadogAgent/Production/api_key: (SecureString) "••••••••••••0f83"
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
api_key: "ENC[agent_secret:/DatadogAgent/Production/api_key]" 
```

**AWS IAM User Access Key with SSM parameter_path recursive fetch**

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
    parameter_path: /DatadogAgent/Production
```

**AWS IAM User with sts:AssumeRole and specific SSM parameters**

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
    parameters:
      - /DatadogAgent/Production/api_key
```
