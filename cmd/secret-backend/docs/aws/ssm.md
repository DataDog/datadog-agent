# AWS Systems Manager Parameter Store Backend

> [Datadog Agent Secrets](https://docs.datadoghq.com/agent/guide/secrets-management/?tab=linux) using [AWS Systems Manager Parameter Store](https://docs.aws.amazon.com/systems-manager/latest/userguide/systems-manager-parameter-store.html)

## Configuration

### IAM Permission Policy (if using an Instance Profile)

Create a similar IAM Permission Policy as the example below to allow resources (EC2, ECS, etc. instances) to access your specified secrets. Please refer to the [AWS SSM official documentation](https://docs.aws.amazon.com/systems-manager/) for more details on allowing resources to access secrets. 

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
				"ssm:GetParameters",
				"ssm:GetParameter",
				"ssm:GetParametersByPath",
				"ssm:DescribeParameters"
			],
      "Resource": [
        "arn:aws:ssm:${Region}:${Account}:parameter/${ParameterPathWithoutLeadingSlash}"
      ]
    }
  ]
}

```

You can use a wildcard when specifying the parameter path `Resource` (for example, `datadog/*` for all resources within in the `datadog` folder).

This is just one step in setting up the Instance Profile. Refer to the [Instance Profile Instructions](https://github.com/DataDog/datadog-secret-backend/blob/main/docs/aws/README.md#instance-profile-instructions) in the AWS README to complete the setup.

### Backend Settings

| Setting | Description |
| --- | --- |
| backend_type | Backend type |
| parameter_path| SSM parameters prefix, recursive |
| parameters | List of individual SSM parameters |

## Backend Configuration

Ensure that you have followed the instructions specified in the general [aws README](https://github.com/DataDog/datadog-secret-backend/blob/main/docs/aws/README.md) to avoid hardcoding any confidential information in your config file.

The backend configuration for AWS SSM Parameter Store secrets has the following pattern:

```yaml
---
secret_backend_type: aws.ssm
secret_backend_config:
  parameters: 
    - /DatadogAgent/Production/ParameterKey1
    - /DatadogAgent/Production/ParameterKey2
    - /DatadogAgent/Production/ParameterKey3
  parameter_path: /DatadogAgent/Production
  aws_session:
    aws_region: us-east-1
```

**secret_backend_type** must be set to `aws.ssm` and either or both of **parameter_path** and **parameters** must be provided in each backend configuration.

The backend secret is referenced in your Datadog Agent configuration files using the **ENC** notation.

```yaml
# /etc/datadog-agent/datadog.yaml

api_key: ENC[{parameter_full_path}]

```

AWS System Manager Parameter store supports a heirachical model. Parameters can be specified individually using **parameters**, or recursively fetched using a matching
 prefix path with **parameter_path**. For example, assuming the AWS System Manager Parameter Store paths

```sh
/DatadogAgent/Production/ParameterKey1 = ParameterStringValue1
/DatadogAgent/Production/ParameterKey2 = ParameterStringValue2
/DatadogAgent/Production/ParameterKey3 = ParameterStringValue3
```

The parameters can be fetched using **parameter_path**:

```yaml
# /etc/datadog-agent/datadog.yaml
---
secret_backend_type: aws.ssm
secret_backend_config:
  parameter_path: /DatadogAgent/Production
  aws_session:
    aws_region: us-east-1
```

or fetched using **parameters**:

```yaml
# /etc/datadog-agent/datadog.yaml
---
secret_backend_type: aws.ssm
secret_backend_config:
  parameters: 
    - /DatadogAgent/Production/ParameterKey1
    - /DatadogAgent/Production/ParameterKey2
    - /DatadogAgent/Production/ParameterKey3
  aws_session:
    aws_region: us-east-1
```

and finally accessed in the Datadog Agent:

```yaml
# /etc/datadog-agent/datadog.yaml
property1: "ENC[/DatadogAgent/Production/ParameterKey1]"
property2: "ENC[/DatadogAgent/Production/ParameterKey2]"
property3: "ENC[/DatadogAgent/Production/ParameterKey3]"
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
api_key: "ENC[/DatadogAgent/Production/api_key]" 
```

**AWS IAM User Access Key with SSM parameter_path recursive fetch**

```yaml
# /etc/datadog-agent/datadog.yaml
---
secret_backend_type: aws.ssm
secret_backend_config:
parameter_path: /DatadogAgent/Production
aws_session:
  aws_region: us-east-1
```
