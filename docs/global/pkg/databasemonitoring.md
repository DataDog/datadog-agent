> **TL;DR:** `pkg/databasemonitoring` provides AWS RDS and Aurora auto-discovery logic that discovers database instances by AWS tags and feeds them to the autodiscovery system so the appropriate DBM checks are scheduled without manual configuration.

# pkg/databasemonitoring

## Purpose

`pkg/databasemonitoring` contains the agent-side logic for **Database Monitoring (DBM) auto-discovery**. Its only current sub-package, `aws/`, discovers RDS and Aurora database instances running in the same AWS account and region as the agent, then feeds them to the autodiscovery system so the appropriate DBM checks (postgres, mysql) are scheduled automatically — without requiring manual `conf.d` entries for every instance.

The package is only compiled with the `ec2` build tag because it assumes the agent is running on an EC2 instance and uses the EC2 instance identity endpoint to determine the AWS region.

## Key elements (`pkg/databasemonitoring/aws/`)

### Key types

#### Types

| Type | Description |
|------|-------------|
| `Instance` | Represents a single RDS or Aurora database instance. Fields: `ID`, `ClusterID`, `Endpoint`, `Port`, `IamEnabled`, `Engine`, `DbName`, `GlobalViewDb`, `DbmEnabled`. |
| `AuroraCluster` | Groups `[]*Instance` belonging to the same Aurora cluster ID. |
| `Client` | Wraps `aws-sdk-go-v2`'s RDS service client. Created via `NewRdsClient`. |
| `Config` | Discovery configuration: `Enabled`, `DiscoveryInterval` (seconds), `QueryTimeout` (seconds), `Tags` (filter), `DbmTag` (opt-in tag), `GlobalViewDbTag`, `Region`. |
| `RdsClient` | Interface satisfied by `*Client`. Mocked in tests via `rdsclient_mockgen.go`. |

### Key functions

#### Constructors and configuration

| Symbol | Description |
|--------|-------------|
| `NewRdsClient(region string) (*Client, string, error)` | Creates an AWS RDS client. If `region` is empty, auto-discovers from the EC2 instance identity API. Loads AWS credentials from environment variables or shared config files via `aws-sdk-go-v2`. |
| `NewAuroraAutodiscoveryConfig() (Config, error)` | Reads `database_monitoring.autodiscovery.aurora.*` from `datadog.yaml`. |
| `NewRdsAutodiscoveryConfig() (Config, error)` | Reads `database_monitoring.autodiscovery.rds.*` from `datadog.yaml`. |

#### Discovery methods on `*Client`

| Method | Description |
|--------|-------------|
| `GetAuroraClustersFromTags(ctx, tags []string) ([]string, error)` | Lists Aurora clusters (mysql + postgresql engines) and returns the identifiers of those whose AWS tags match all entries in `tags`. Tags are looked up on the *cluster* — not on instances — because Aurora auto-scaling events do not propagate tags to new instances. Paginated. |
| `GetAuroraClusterEndpoints(ctx, dbClusterIdentifiers []string, config Config) (map[string]*AuroraCluster, error)` | For each cluster ID, lists its `available` DB instances and returns them grouped by cluster. Note: currently limited to 100 instances per cluster (not paginated). |
| `GetRdsInstancesFromTags(ctx, config Config) ([]Instance, error)` | Lists all RDS instances with `postgres`, `mysql`, `aurora-mysql`, or `aurora-postgresql` engines, filtering by the AWS tags in `config.Tags`. Paginated. |

#### Helper methods on `Instance`

| Method | Description |
|--------|-------------|
| `(*Instance).Digest(checkType, clusterID string) string` | Returns a stable FNV-64 hash of `(checkType, clusterID, Endpoint, Port, Engine, IamEnabled)` as a hex string. Used by autodiscovery listeners to detect configuration changes and avoid re-scheduling unchanged instances. |

### Configuration and build flags

All files carry `//go:build ec2`. Configuration keys are under `database_monitoring.autodiscovery.aurora.*` and `database_monitoring.autodiscovery.rds.*` in `datadog.yaml`.

#### Internal helpers

- `makeInstance(db types.DBInstance, config Config) (*Instance, error)` — constructs an `Instance` from an AWS SDK `DBInstance`. Resolves the default `DbName` from the engine type when the DB does not have one. Sets `DbmEnabled` if the instance carries `config.DbmTag`.
- `containsTags(clusterTags, providedTags)` — checks that all `providedTags` (as `key:value` strings, case-insensitive) are present in `clusterTags`.
- `dbNameFromEngine(engine)` — maps engine names to default database names (`postgres` or `mysql`).

## Build flag

All files in `pkg/databasemonitoring/aws/` carry `//go:build ec2`. The package is therefore only included in agent binaries built with `-tags ec2`, which is the standard tag for EC2-aware builds.

## Configuration reference

| Key | Description |
|-----|-------------|
| `database_monitoring.autodiscovery.aurora.enabled` | Enable Aurora auto-discovery. |
| `database_monitoring.autodiscovery.aurora.discovery_interval` | Polling interval in seconds. |
| `database_monitoring.autodiscovery.aurora.query_timeout` | AWS API call timeout in seconds. |
| `database_monitoring.autodiscovery.aurora.tags` | AWS tags to filter clusters (e.g. `["env:prod"]`). |
| `database_monitoring.autodiscovery.aurora.dbm_tag` | Tag that marks a cluster as DBM-enabled (e.g. `datadoghq.com/dbm:true`). |
| `database_monitoring.autodiscovery.aurora.global_view_db_tag` | AWS tag key whose value names the global-view database. |
| `database_monitoring.autodiscovery.aurora.region` | AWS region; auto-detected from EC2 metadata if blank. |
| `database_monitoring.autodiscovery.rds.*` | Same keys for standalone RDS (non-Aurora) discovery. |

## Usage

The autodiscovery listeners in `comp/core/autodiscovery/listeners/` consume this package:

- `DBMAuroraListener` — polls on a ticker. On each tick it calls `GetAuroraClustersFromTags` then `GetAuroraClusterEndpoints`, diffs the result against `services`, and sends `newService`/`delService` events to the autodiscovery engine. Each discovered instance becomes a `DBMAuroraService` that exposes an integration config for the `postgres` or `mysql` DBM check.
- `DBMRdsListener` — same pattern using `GetRdsInstancesFromTags`. Each instance becomes a `DBMRdsService`.

Both listeners use `Instance.Digest` to detect whether a previously-seen instance has changed (e.g. port or IAM setting changed) before issuing a service replacement event.

```go
// Typical listener bootstrap (simplified from dbm_aurora.go)
config, _  := aws.NewAuroraAutodiscoveryConfig()
client, region, _ := aws.NewRdsClient(config.Region)
// on every tick:
clusterIDs, _ := client.GetAuroraClustersFromTags(ctx, config.Tags)
clusters, _   := client.GetAuroraClusterEndpoints(ctx, clusterIDs, config)
// diff clusters against current services and emit add/remove events
```

## Cross-references

| Related package / component | Relationship |
|-----------------------------|--------------|
| [`pkg/util/aws`](util/aws.md) | `NewRdsClient` auto-detects the AWS region using the same IMDS instance-identity document endpoint exposed by `pkg/util/aws/creds.GetAWSRegion`. When `config.Region` is blank, the package queries IMDS directly via `aws-sdk-go-v2`'s built-in EC2 metadata provider rather than going through `pkg/util/aws/creds` directly, but the same `ec2` build tag and IMDS endpoint are involved. |
| [`comp/core/autodiscovery`](../comp/core/autodiscovery.md) | `DBMAuroraListener` and `DBMRdsListener` implement the `ServiceListener` interface and plug into the autodiscovery engine. Each discovered `Instance` is wrapped in a `DBMAuroraService` / `DBMRdsService` that produces an `integration.Config` for the `postgres` or `mysql` DBM check. Autodiscovery schedules and unschedules these configs via its `MetaScheduler` → `CheckScheduler` path. |
