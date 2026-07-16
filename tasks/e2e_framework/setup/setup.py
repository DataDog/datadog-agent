import json
import os
import os.path
import shutil
from pathlib import Path

from invoke.context import Context
from invoke.exceptions import Exit, UnexpectedExit
from invoke.tasks import task

from tasks.e2e_framework import config as e2e_config
from tasks.e2e_framework import doc
from tasks.e2e_framework.tool import (
    debug,
    error,
    get_aws_cmd,
    get_pulumi_run_folder,
    info,
    is_windows,
    warn,
)


def _force_cleanup(
    ctx: Context,
    config_path: str | None = None,
    account: str | None = None,
    with_azure: bool = False,
    with_gcp: bool = False,
) -> None:
    """
    Delete the config file and the SSH keys that setup would auto-generate:
    the AWS keypair (from EC2 and local disk) always; Azure and GCP local files
    when the matching --with-* flag is active.

    Uses computed default paths — not the existing config — so it targets exactly
    what the next setup run would create.
    """
    import getpass

    from tasks.e2e_framework.config import get_full_profile_path
    from tasks.e2e_framework.setup.aws import DEFAULT_AWS_ACCOUNT, _default_keypair_name
    from tasks.e2e_framework.setup.ssh_keys import default_key_paths, ssh_agent_supported

    info("🧹 Force-cleaning existing config and SSH keys...")

    def _remove_key(priv: Path, pub: Path) -> None:
        """Remove key from ssh-agent then delete both local files."""
        if ssh_agent_supported() and pub.is_file():
            try:
                ctx.run(f'ssh-add -d "{pub}"', hide=True, warn=True)
                info(f"✓ Removed key from ssh-agent: {pub.name}")
            except Exception:
                pass
        for p in [priv, pub]:
            if p.is_file():
                p.unlink()
                info(f"✓ Deleted {p}")

    user = getpass.getuser()
    effective_account = account or DEFAULT_AWS_ACCOUNT

    # AWS: remove from agent, delete local files, then delete EC2 keypair
    _remove_key(*default_key_paths(effective_account, user))
    keypair_name = _default_keypair_name(effective_account, user)
    try:
        cmd = get_aws_cmd(
            f'ec2 delete-key-pair --key-name "{keypair_name}"',
            use_aws_vault=True,
            aws_account=effective_account,
        )
        out = ctx.run(cmd, warn=True, hide="stdout")
        if out and out.exited == 0:
            info(f"✓ Deleted AWS keypair '{keypair_name}'")
        else:
            warn(f"AWS keypair '{keypair_name}' not found or credentials expired — skipping")
    except Exception as e:
        warn(f"Could not delete AWS keypair '{keypair_name}': {e}")

    # Azure: remove from agent and delete local files when --with-azure is active
    if with_azure:
        _remove_key(*default_key_paths(effective_account, user, provider="azure", key_type="ed25519"))

    # GCP: remove from agent and delete local files when --with-gcp is active
    if with_gcp:
        _remove_key(*default_key_paths(effective_account, user, provider="gcp", key_type="ed25519"))

    # Delete config file last
    full_config_path = Path(get_full_profile_path(config_path))
    if full_config_path.is_file():
        full_config_path.unlink()
        info(f"✓ Deleted config {full_config_path}")


@task(
    help={
        "config_path": doc.config_path,
        "interactive": doc.interactive,
        "debug": doc.debug,
        "with_azure": doc.with_azure,
        "with_gcp": doc.with_gcp,
        "account": doc.account,
        "force": doc.force,
    },
    default=True,
)
def setup(
    ctx: Context,
    config_path: str | None = None,
    interactive: bool | None = True,
    debug: bool | None = False,
    with_azure: bool = False,
    with_gcp: bool = False,
    account: str | None = None,
    force: bool = False,
) -> None:
    """
    Configure the local environment for E2E tests.

    On the default path this configures AWS only (the cloud the vast majority of
    E2E tests target) and asks at most one question (the GitHub team tag for
    resource attribution). Pass --with-azure / --with-gcp to also configure
    those providers.
    """
    from tasks.e2e_framework import config
    from tasks.e2e_framework.setup.agent import setup_agent_config
    from tasks.e2e_framework.setup.aws import setup_aws_config
    from tasks.e2e_framework.setup.config import check_config
    from tasks.e2e_framework.setup.pulumi import install_pulumi, pulumi_version, setup_pulumi_config

    # AWS CLI is the only hard prereq on the default path.
    if not shutil.which("aws"):
        error("AWS CLI not found, please install it: https://aws.amazon.com/cli/")
        raise Exit(code=1)

    if with_azure and not shutil.which("az"):
        error("Azure CLI not found, please install it: https://learn.microsoft.com/en-us/cli/azure/install-azure-cli")
        raise Exit(code=1)
    if with_gcp and not shutil.which("gcloud"):
        error("Gcloud CLI not found, please install it: https://cloud.google.com/sdk/docs/install")
        raise Exit(code=1)

    if with_gcp:
        from tasks.e2e_framework.setup.gcp import install_gcloud_auth_plugin

        install_gcloud_auth_plugin(ctx)

    pulumi_ver, pulumi_up_to_date = pulumi_version(ctx)
    if pulumi_up_to_date:
        info(f"✓ Pulumi is up to date: {pulumi_ver}")
    else:
        install_pulumi(ctx)

    with ctx.cd(get_pulumi_run_folder()):
        ctx.run("pulumi --non-interactive plugin install", hide=True)
        ctx.run("pulumi login --local", hide=True)
    info("✓ Pulumi plugins installed; local backend configured")

    try:
        cfg = config.get_local_config(config_path)
    except Exception:
        cfg = config.Config.model_validate({})

    if force:
        _force_cleanup(ctx, config_path=config_path, account=account, with_azure=with_azure, with_gcp=with_gcp)
        cfg = config.Config.model_validate({})

    if interactive:
        info("🤖 Configuring E2E environment...")
        setup_aws_config(ctx, cfg, account=account)
        setup_agent_config(cfg)
        setup_pulumi_config(cfg)

        if with_azure:
            from tasks.e2e_framework.setup.azure import setup_azure_config

            setup_azure_config(ctx, cfg)
        if with_gcp:
            from tasks.e2e_framework.setup.gcp import setup_gcp_config

            setup_gcp_config(ctx, cfg)

        cfg.save_to_local_config(config_path)

    check_config(cfg)

    if debug:
        debug_env(ctx, config_path=config_path)

    if interactive:
        info("\n✓ Setup complete. Try: dda inv new-e2e-tests.run --targets=./test/new-e2e/examples\n")


@task
def aws_sso(ctx: Context, config_path: str | None = None):
    """
    Setup AWS SSO profile for the agent-sandbox account if it doesn't exist

    Helper mainly here for Windows users who can't use the macos laptop setup script
    """
    from tasks.e2e_framework.config import get_local_config
    from tasks.e2e_framework.setup.aws import setup_aws_sso_config

    try:
        config = get_local_config(config_path)
    except Exception as e:
        error(f"{e}")
        error("Failed to load config")
        raise Exit(code=1) from e

    setup_aws_sso_config(config)


@task
def aws_create_keypair(
    ctx: Context,
    keypair_name: str | None = None,
    key_type: str | None = None,
    private_key_path: str | None = None,
    public_key_path: str | None = None,
    use_aws_vault: bool | None = False,
    aws_account_name: str | None = None,
    config_path: str | None = None,
) -> None:
    """
    Create a new key pair using the AWS CLI, save the key pair to disk, and update the local config.

    This task is interactive and can be run with no options:
      - Prompt user for missing values
      - Confirm overwrite of existing key pair
      - Confirm update of local config
    """

    from tasks.e2e_framework.config import get_local_config
    from tasks.e2e_framework.setup.aws import _aws_create_keypair, update_config_aws_keypair

    try:
        config = get_local_config(config_path)
    except Exception as e:
        error(f"{e}")
        error("Failed to load config")
        raise Exit(code=1) from e

    _aws_create_keypair(
        ctx=ctx,
        config=config,
        keypair_name=keypair_name,
        key_type=key_type,
        private_key_path=private_key_path,
        public_key_path=public_key_path,
        use_aws_vault=use_aws_vault,
        aws_account_name=aws_account_name,
    )

    update_config_aws_keypair(
        config,
        config_path=config_path,
    )


@task
def aws_import_keypair(
    ctx: Context,
    keypair_name: str | None = None,
    private_key_path: str | None = None,
    public_key_path: str | None = None,
    use_aws_vault: bool | None = False,
    aws_account_name: str | None = None,
    config_path: str | None = None,
) -> None:
    """
    Import an existing key pair to AWS and update the config.

    This task is interactive and can be run with no options:
      - Prompt user for missing values
      - Confirm overwrite of existing key pair
      - Confirm update of local config
    """
    from tasks.e2e_framework.config import get_local_config
    from tasks.e2e_framework.setup.aws import _aws_import_keypair, update_config_aws_keypair

    try:
        config = get_local_config(config_path)
    except Exception as e:
        error(f"{e}")
        error("Failed to load config")
        raise Exit(code=1) from e

    _aws_import_keypair(
        ctx=ctx,
        config=config,
        keypair_name=keypair_name,
        private_key_path=private_key_path,
        public_key_path=public_key_path,
        use_aws_vault=use_aws_vault,
        aws_account_name=aws_account_name,
    )

    update_config_aws_keypair(
        config,
        config_path=config_path,
    )


@task(help={"config_path": doc.config_path})
def debug_keys(ctx: Context, config_path: str | None = None):
    """
    Debug E2E and test-infra-definitions SSH keys
    """
    from tasks.e2e_framework.config import get_local_config
    from tasks.e2e_framework.setup.aws import find_matching_ec2_keypair, load_ec2_keypairs
    from tasks.e2e_framework.setup.ssh_keys import (
        check_key,
        get_ssh_keys,
        is_key_encrypted,
        passphrase_decrypts_privatekey,
        ssh_agent_supported,
    )

    if ssh_agent_supported():
        # Ensure ssh-agent is running
        try:
            ctx.run("ssh-add -l", hide=True)
        except UnexpectedExit as e:
            error(f"{e}")
            error("ssh-agent not available or no keys are loaded, please start it and load your keys")
            raise Exit(code=1) from e

    found = False
    keypairs = load_ec2_keypairs(ctx)

    info("Checking for valid SSH key configuration")

    # Get keypair name
    try:
        config = get_local_config(config_path)
    except Exception as e:
        error(f"{e}")
        error("Failed to load config")
        raise Exit(code=1) from e
    if config.configParams.aws is None:
        error("configParams.aws missing from config")
        raise Exit(code=1)
    awsConf = config.configParams.aws
    keypair_name = awsConf.keyPairName or ""

    # lookup configured keypair
    info("Checking configured keypair:")
    debug(f"\taws.keyPairName: {keypair_name}")
    debug(f"\taws.privateKeyPath: {awsConf.privateKeyPath}")
    debug(f"\taws.publicKeyPath: {awsConf.publicKeyPath}")
    for keypair in keypairs:
        if keypair["KeyName"] == keypair_name:
            info("Configured keyPairName found in aws!")
            debug(json.dumps(keypair, indent=4))
            break
    else:
        error(
            "Configured keyPairName missing from aws! Ensure the keypair is uploaded to the correct region and account."
        )
        raise Exit(code=1)
    # check if private key is encrypted
    if awsConf.privateKeyPath and is_key_encrypted(ctx, awsConf.privateKeyPath):
        if awsConf.privateKeyPassword:
            if not passphrase_decrypts_privatekey(ctx, awsConf.privateKeyPath, awsConf.privateKeyPassword):
                error("Private key password is incorrect")
                raise Exit(code=1)
        else:
            # pulumi-command remote.Connection errors if the private key is encrypted and no password is provided
            # and exits with an error before trying any other auth methods.
            # https://github.com/pulumi/pulumi-command/blob/58dda0317f72920537b3a0c9613ce5fed0610533/provider/pkg/provider/remote/connection.go#L81-L93
            if is_windows():
                error(
                    "Private key is encrypted and no password is provided in the config. Pulumi does not support Windows SSH agent."
                )
                info("Remove the passphrase from the key or provide the privateKeyPassword.")
            else:
                error("Private key is encrypted and no password is provided in the config.")
                info(
                    "Remove the privateKeyPath option, or remove the passphrase from the key, or provide the privateKeyPassword."
                )
            raise Exit(code=1)
    if is_windows() and not awsConf.privateKeyPath:
        # https://github.com/pulumi/pulumi-command/blob/58dda0317f72920537b3a0c9613ce5fed0610533/provider/pkg/provider/remote/connection.go#L105-L118
        error("Private key is not provided in the config. Pulumi does not support Windows SSH agent.")
        info("Configure privateKeyPath and provide the privateKeyPassword if the key is encrypted.")
    if not awsConf.privateKeyPath:
        warn("WARNING: privateKeyPath is not configured. You will not be able to decrypt Windows RDP credentials.")

    configuredKeyInfo = {}
    for keyname in ["privateKeyPath", "publicKeyPath"]:
        keypair_path = getattr(awsConf, keyname)
        if keypair_path is None:
            continue
        keyinfo, keypair = find_matching_ec2_keypair(ctx, keypairs, keypair_path)
        if keyinfo is not None:
            configuredKeyInfo[keyname] = keyinfo
        if keyinfo is not None and keypair is not None:
            info(f"Configured {keyname} found in aws!")
            debug(json.dumps(keypair, indent=4))
            check_key(ctx, keyinfo, keypair, keypair_name)
            found = True
        else:
            if keyinfo is not None and keyinfo.is_rsa_pubkey:
                debug(
                    f"NOTICE: {keyname} is an RSA public key, these cannot be matched to aws keys. To avoid errors, ensure that the privateKeyPath is found in AWS and the privateKeyPath and publicKeyPath fingerprints match."
                )
            else:
                warn(f"WARNING: Configured {keyname} missing from aws!")

    # Check that private and public keys match
    if "privateKeyPath" in configuredKeyInfo and "publicKeyPath" in configuredKeyInfo:
        for privf in configuredKeyInfo["privateKeyPath"].fingerprint:
            for pubf in configuredKeyInfo["publicKeyPath"].fingerprint:
                if privf == pubf:
                    info("privateKeyPath and publicKeyPath fingerprints match!")
                    break
            else:
                continue
            break
        else:
            warn("WARNING: privateKeyPath and publicKeyPath fingerprints do not match!")

    print()

    info("Checking if any SSH key is configured in aws")

    # check all keypairs
    for keypath in get_ssh_keys():
        try:
            keyinfo, keypair = find_matching_ec2_keypair(ctx, keypairs, keypath)
        except (ValueError, UnexpectedExit) as e:
            if 'not a valid ssh key' in str(e):
                continue
            warn(f'WARNING: {e}')
            continue
        if keyinfo is not None and keypair is not None:
            info(f"Found '{keypair['KeyName']}' matches: {keypath}")
            debug(json.dumps(keypair, indent=4))
            check_key(ctx, keyinfo, keypair, keypair_name)
            print()
            found = True

    if not found:
        error("No matching keypair found in aws!")
        info(
            "If this is unexpected, confirm that your aws credential's region matches the region you uploaded your key to."
        )
        raise Exit(code=1)


@task(name="debug", help={"config_path": doc.config_path})
def debug_env(ctx, config_path: str | None = None):
    """
    Debug E2E and test-infra-definitions required tools and configuration
    """
    # check pulumi found
    try:
        out = ctx.run("pulumi version", hide=True)
    except UnexpectedExit as e:
        error(f"{e}")
        error("Pulumi CLI not found, please install it: https://www.pulumi.com/docs/get-started/install/")
        raise Exit(code=1) from e
    info(f"Pulumi version: {out.stdout.strip()}")

    # Check pulumi credentials
    try:
        out = ctx.run("pulumi whoami", hide=True)
    except UnexpectedExit as e:
        error("No pulumi credentials found")
        info("Please login, e.g. pulumi login --local")
        raise Exit(code=1) from e

    # check awscli version
    out = ctx.run("aws --version", hide=True)
    if not out.stdout.startswith("aws-cli/2"):
        error(f"Detected invalid awscli version: {out.stdout}")
        info(
            "Please remove the current version and install awscli v2: https://docs.aws.amazon.com/cli/latest/userguide/cliv2-migration-instructions.html"
        )
        raise Exit(code=1)
    info(f"AWS CLI version: {out.stdout.strip()}")

    # check aws-vault found
    try:
        out = ctx.run("aws-vault --version", hide=True)
    except UnexpectedExit as e:
        error(f"{e}")
        error("aws-vault not found, please install it")
        raise Exit(code=1) from e
    info(f"aws-vault version: {out.stderr.strip()}")

    print()

    # check .aws/config exists and contains the expected profile for the
    # configured account (falls back to agent-sandbox, the historical default).
    try:
        cfg = e2e_config.get_local_config(config_path)
        account = cfg.get_aws().account or 'agent-sandbox'
    except Exception:
        account = 'agent-sandbox'
    expected_profile = f'sso-{account}-account-admin'
    aws_conf_path = Path.home().joinpath(".aws", "config")
    if not os.path.isfile(aws_conf_path):
        error(f"Missing aws config file: {aws_conf_path}")
        info("Please run `inv setup.aws-sso` or create it manually with `aws configure sso`.")
        raise Exit(code=1)
    with open(aws_conf_path) as f:
        conf = f.read()
        if expected_profile not in conf:
            error(f"Profile {expected_profile} not found in aws config file: {aws_conf_path}")
            info("Please run `inv setup.aws-sso` or create it manually with `aws configure sso`.")
            raise Exit(code=1)

    # Show AWS account info
    info("Logged-in aws account info:")
    if os.environ.get("AWS_PROFILE"):
        info(f"\tAWS_PROFILE={os.environ.get('AWS_PROFILE')}")
        region = os.environ.get("AWS_REGION")
        if not region:
            raise Exit("Missing env var AWS_REGION, please set var", 1)
        info(f"\tAWS_REGION={region}")
    else:
        for env in ["AWS_VAULT", "AWS_REGION"]:
            val = os.environ.get(env, None)
            if val is None:
                raise Exit(f"Missing env var {env}, please login with awscli/aws-vault or set AWS_PROFILE", 1)
            info(f"\t{env}={val}")

    # Check if aws creds are valid
    try:
        out = ctx.run("aws sts get-caller-identity", hide=True)
    except UnexpectedExit as e:
        error(f"{e}")
        error("No AWS credentials found or they are expired, please configure and/or login")
        raise Exit(code=1) from e

    print()

    # Check the same profile is registered with aws-vault.
    out = ctx.run("aws-vault list", hide=True)
    if expected_profile not in out.stdout:
        warn(f"WARNING: expected profile {expected_profile} missing from aws-vault. Some invoke tasks may fail.")
        print()

    debug_keys(ctx, config_path=config_path)
