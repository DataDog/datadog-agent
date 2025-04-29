# Windows Build Scripts

The scripts in this directory are entrypoints to multi-step tasks that are performed by our CI jobs. Many scripts use the `Invoke-BuildScript` helper function to set up the build environment and install dependencies before executing their unique steps.

## General Usage

Each script is intended to function in the CI pipeline as well as locally, and has options to facilitate each use case. Run `Get-Help <script>` to get specific information and examples for each script.

### Common Parameters

The `Invoke-BuildScript` function provides several parameters that are shared across the build scripts:

- `BuildOutOfSource`: Copies the entire source tree to a new directory. Default is `$false`. Use this option in the CI to keep the job directory clean and avoid conflicts/stale data. Use this option in Hyper-V based containers to improve build performance.
- `InstallDeps`: Specifies whether to install dependencies (Python requirements, Go dependencies, etc.). Default is `$true`.
- `CheckGoVersion`: Specifies whether to check the Go version. If not provided, it defaults to the value of the environment variable `GO_VERSION_CHECK` or `$true` if the environment variable is not set.

### CI Pipeline

- The CI runs in a fresh container, so it must use `-InstallDeps $true` every time to ensure all dependencies are installed.
- The CI uses `-BuildOutOfSource $true` to keep the job directory clean and avoid conflicts/stale data.

### Local Development

- Locally, you should only need to run with `-InstallDeps $true` once to install the necessary dependencies. Subsequent runs can use `-InstallDeps $false` to save time.
- You should not need to use `-BuildOutOfSource $true` when running scripts locally.
- You may not need to use these scripts at all. Instead, consider running the smaller components directly. For example, you can run `go test` on the package you are working on instead of running all unit tests using `Invoke-UnitTests.ps1`.

## Scripts

Below are quick details for a few primary scripts. There are other scripts in this directory but they are similar enough to these and also have their own `Get-Help` documentation.

### Invoke-AgentPackages

The `Invoke-AgentPackages.ps1` script runs the complete omnibus build plus the MSI and OCI packages.

Inner components to consider using instead
- `dda inv agent.build`
- `dda inv msi.build`

#### Additional Parameters

- `ReleaseVersion`: Specifies the release version of the build. Default is the value of the environment variable `RELEASE_VERSION`.
- `Flavor`: Specifies the flavor of the agent. Default is the value of the environment variable `AGENT_FLAVOR`.

### Invoke-UnitTests

The `Invoke-UnitTests.ps1` script runs the unit tests for the Datadog Agent.

#### Additional Parameters

- `UploadCoverage`: Specifies whether to upload test coverage results. Only works in the CI. Default is `$false`.
