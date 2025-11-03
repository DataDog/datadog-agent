import json
from collections import defaultdict

from invoke.tasks import task


@task
def fetch_latest_amis(ctx):
    """
    Will fetch the latest AMIs for the given platforms.
    This is based on resources/aws/platforms_params.json.

    Args:
        merge (bool): Whether to merge the new AMIs with the existing ones.
    """

    with open("resources/aws/platforms_params.json") as f:
        platforms_params = json.load(f)

    new_amis = defaultdict(lambda: defaultdict(dict))
    error = False
    print('Fetching new AMIs...')
    for os, archs in platforms_params.items():
        for arch, versions in archs.items():
            for version, param in versions.items():
                try:
                    res = ctx.run(
                        f"aws-vault exec sso-agent-qa-account-admin -- aws ssm get-parameter --name {param} --query 'Parameter.Value' --output text",
                        hide=True,
                    ).stdout.strip()
                    new_amis[os][arch][version] = res
                except Exception:
                    error = True
                    print(f"ERROR: Failed to fetch AMI for {os} {arch} {version} (param: {param})")

    print('Fetched new AMIs:')
    print(json.dumps(new_amis, indent=4))

    if error:
        raise RuntimeError("Failed to fetch some AMIs")
