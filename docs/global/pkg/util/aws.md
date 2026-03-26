# pkg/util/aws

## Purpose

`pkg/util/aws` provides helpers for interacting with AWS from within the agent.
The top-level package is currently empty aside from sub-packages; all logic lives under `creds/`.

### pkg/util/aws/creds

`creds` is responsible for two things:

1. **EC2 credential retrieval** — fetching temporary IAM credentials from the EC2 Instance Metadata Service (IMDS) so other components can sign AWS API calls without shipping long-lived keys.
2. **AWS environment detection** — deciding whether the agent is running on EC2 and resolving the current region, using a layered strategy (environment variables → IMDS).

The sub-package `creds/internal` (package `ec2internal`) is an internal-only layer that owns all direct IMDS HTTP communication. It implements IMDSv1/v2 token management, the HTTP request helper, and the instance-identity document fetcher. It is intentionally duplicated from `pkg/util/ec2/internal` because Go's `internal` visibility rules would otherwise prevent importing it.

## Key elements

### creds (public API, `pkg/util/aws/creds`)

| Symbol | Build tag | Description |
|--------|-----------|-------------|
| `SecurityCredentials` struct | — | Holds `AccessKeyID`, `SecretAccessKey`, and `Token` (session token). |
| `GetSecurityCredentials(ctx) (*SecurityCredentials, error)` | `ec2` | Queries IMDS at `/iam/security-credentials/<role>` to obtain temporary credentials for the attached IAM role. Returns an error stub when compiled without `ec2`. |
| `HasAWSCredentialsInEnvironment() bool` | `ec2` | Returns `true` when `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` are both set. |
| `IsRunningOnAWS(ctx) bool` | `ec2` | Returns `true` if env credentials are present **or** the instance-identity document is reachable via IMDS. Always `false` without `ec2`. |
| `GetAWSRegion(ctx) (string, error)` | `ec2` | Resolves the current region by checking `AWS_REGION` → `AWS_DEFAULT_REGION` → IMDS identity document. Always errors without `ec2`. |

### creds/internal (ec2internal)

| Symbol | Description |
|--------|-------------|
| `Ec2IMDSVersionConfig` enum (`ImdsV1`, `ImdsAllVersions`, `ImdsV2`) | Controls which IMDS version(s) to use when making a request. |
| `UseIMDSv2() Ec2IMDSVersionConfig` | Reads `ec2_prefer_imdsv2` / `ec2_imdsv2_transition_payload_enabled` from agent config and returns the appropriate version policy. |
| `DoHTTPRequest(ctx, url, versions, updateSource)` | Core IMDS HTTP helper. Fetches an IMDSv2 token when permitted, falls back to v1, and optionally updates the global `CurrentMetadataSource` counter. |
| `GetInstanceIdentity(ctx)` | Fetches and parses the instance-identity document (`/latest/dynamic/instance-identity/document/`), returning `EC2Identity{Region, InstanceID, AccountID}`. |
| `GetToken(ctx)` | Fetches a short-lived IMDSv2 session token via HTTP `PUT` to the token endpoint. |
| `SetCloudProviderSource(source int)` / `GetSourceName()` | Thread-safe bookkeeping for the "best" metadata source seen so far, exposed in the inventories payload. |
| `MetadataURL`, `TokenURL`, `InstanceIdentityURL` | Package-level vars (overridable in tests) pointing to the IMDS link-local address. |

### Build tags

| Tag | Effect |
|-----|--------|
| `ec2` | Enables IMDS-backed implementations of `GetSecurityCredentials`, `IsRunningOnAWS`, and `GetAWSRegion`. Without this tag, all three return stubs/errors. |

## Usage

### Delegated auth (SigV4 signing)

`comp/core/delegatedauth/api/cloudauth/aws/aws.go` is the primary consumer. It implements `AWSAuth`, which calls `creds.GetSecurityCredentials` (falling back to env vars) and then uses the AWS SDK v2 `v4.Signer` to produce a signed STS `GetCallerIdentity` request. The resulting proof is forwarded to the Datadog API to authenticate the agent's AWS identity.

```go
import "github.com/DataDog/datadog-agent/pkg/util/aws/creds"

sc, err := creds.GetSecurityCredentials(ctx)
if err != nil {
    // handle; agent may still work via env vars
}
// use sc.AccessKeyID, sc.SecretAccessKey, sc.Token
```

### Region detection

```go
region, err := creds.GetAWSRegion(ctx)
// tries AWS_REGION, then AWS_DEFAULT_REGION, then IMDS
```

### AWS presence check

```go
if creds.IsRunningOnAWS(ctx) {
    // enable AWS-specific code paths
}
```

## Related packages

| Package / component | Relationship |
|---|---|
| [`pkg/util/ec2`](ec2.md) | The primary AWS package for IMDS-based metadata (hostname, instance ID, tags, network). `pkg/util/aws/creds` is intentionally parallel and focuses on IAM credential retrieval and region detection. The two packages **do not import each other**; `creds/internal` duplicates the IMDS HTTP layer (`ec2internal`) because Go `internal` visibility rules prevent sharing it. |
| [`pkg/util/fargate`](fargate.md) | When running on ECS or EKS Fargate, the agent is still on AWS infrastructure. `pkg/util/fargate` detects the specific Fargate orchestrator variant while `pkg/util/aws/creds` handles any IAM credential needs that apply regardless of orchestrator. |
| [`comp/core/ipc`](../../comp/core/ipc.md) | `comp/core/delegatedauth/api/cloudauth/aws` (the primary consumer of this package) calls `creds.GetSecurityCredentials` and then uses the AWS SDK v2 SigV4 signer to produce a signed STS `GetCallerIdentity` request. The resulting proof is forwarded to the Datadog API via the IPC-authenticated HTTP client to authenticate the agent's AWS identity. |
