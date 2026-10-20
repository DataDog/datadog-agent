"""
Registry credentials for the images a locally provisioned cluster pulls.

On AWS the default Agent images and the test workloads all come from the agent-qa ECR
pull-through cache, so that registry always needs a token. An Agent image pushed to the
agent-sandbox ECR needs a second one. The framework takes them as comma-separated lists
(see `DockerConfigJSON` in test/e2e-framework/common/utils/kubernetes.go).
"""

from invoke.context import Context

AGENT_QA_ECR = "669783387624.dkr.ecr.us-east-1.amazonaws.com"
SANDBOX_ECR = "376334461865.dkr.ecr.us-east-1.amazonaws.com"
SANDBOX_ECR_PROFILE = "sso-agent-sandbox-account-admin-8h"

# (registry, username, password)
Credential = tuple[str, str, str]


def agent_qa_ecr_credential(ctx: Context) -> Credential | None:
    """Mint an agent-qa ECR token, or warn and return None when the profile can't."""
    from tasks.e2e_framework.setup.aws import DEFAULT_AWS_REGION, ECR_CACHE_PROFILE

    res = ctx.run(
        f"aws-vault exec {ECR_CACHE_PROFILE} -- aws ecr get-login-password --region {DEFAULT_AWS_REGION}",
        hide=True,
        warn=True,
    )
    if not res or res.exited != 0:
        print(
            f"WARNING: Could not get ECR password for agent-qa account from the '{ECR_CACHE_PROFILE}' profile. "
            "Run `dda inv -- e2e.setup` to configure it. Tests pulling images from agent-qa ECR are likely to fail."
        )
        return None
    return AGENT_QA_ECR, "AWS", res.stdout.strip()


def sandbox_ecr_credential(ctx: Context, *images: str | None) -> Credential | None:
    """Mint an agent-sandbox ECR token when one of the images lives there."""
    if not any(image and image.startswith(f"{SANDBOX_ECR}/") for image in images):
        return None
    password = ctx.run(
        f"aws-vault exec {SANDBOX_ECR_PROFILE} -- aws ecr get-login-password --region us-east-1",
        hide=True,
    ).stdout.strip()
    return SANDBOX_ECR, "AWS", password
