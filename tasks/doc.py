base_image: str = "base Docker image to extract agent python library. defaults to latest release"
target_image: str = "target image to build (agent, cluster-agent), defaults to 'agent'"
process_agent: str = "includes the process agent if set to True, defaults to False"
trace_agent: str = "includes the trace agent if set to True, defaults to False"
system_probe: str = "includes the system probe if set to True, defaults to False"
security_agent: str = "includes the security agent if set to True, defaults to False"
trace_loader: str = "includes trace loader if set to True, defaults to False"
privateactionrunner: str = "includes private action runner if set to True, defaults to False"
push: str = "push the image to repository given by '--target-image' if set to True, defaults to False"
race: str = "turn on go build race detector (for dev and tests only)"
signed_pull: str = "enables Docker Content Trust when building final image"
arch: str = "destination image architecture (aarch64, arm64, amd64, x86_64), to be specified ONLY if autodetection failed, defaults to your local system CPU arch"
development: str = "enables Delve, adds agent libraries to LD_LIBRARY_PATH, for development purpose"
push_to_smp: str = "additionally push the built image to the single-machine-performance ECR, tagged the way the single_machine_performance-full-amd64-a7 CI job does, so an SMP run can use it for analysis. Requires --smp-account-id (or SMP_ACCOUNT_ID)"
smp_account_id: str = (
    "AWS account id owning the SMP ECR registry. Defaults to $SMP_ACCOUNT_ID. Required with --push-to-smp"
)
smp_team_id: str = "comma-separated SMP team id(s); the ECR repository is '<team-id>-agent'. Defaults to $SMP_TEAM_ID, else the team ids the CI job publishes to"
smp_region: str = "AWS region of the SMP ECR registry, defaults to 'us-west-2'"
smp_aws_profile: str = (
    "AWS named profile used to log in to the SMP ECR registry, defaults to 'single-machine-performance'"
)
smp_sha: str = "commit sha used in the image tag. SMP looks images up by commit sha, so this must match the sha you want analyzed. Defaults to the current HEAD sha"
