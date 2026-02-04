import json
import os
import os.path
import shutil
from pathlib import Path

from invoke.context import Context
from invoke.exceptions import Exit, UnexpectedExit
from invoke.tasks import task

from tasks.e2e_framework import doc
from tasks.e2e_framework.tool import (
    debug,
    error,
    get_pulumi_run_folder,
    info,
    is_windows,
    warn,
)


@task(help={"config_path": doc.config_path, "interactive": doc.interactive, "debug": doc.debug}, default=True)
def setup(
    ctx: Context, config_path: str | None = None, interactive: bool | None = True, debug: bool | None = False
) -> None:
    """
    Setup a local environment, interactively by default
    """
    from tasks.e2e_framework import config
    from tasks.e2e_framework.setup.agent import setup_agent_config
    from tasks.e2e_framework.setup.aws import setup_aws_config
    from tasks.e2e_framework.setup.azure import setup_azure_config
    from tasks.e2e_framework.setup.config import check_config
    from tasks.e2e_framework.setup.gcp import install_gcloud_auth_plugin, setup_gcp_config
    from tasks.e2e_framework.setup.pulumi import install_pulumi, pulumi_version, setup_pulumi_config

    # Ensure aws cli is installed
    if not shutil.which("aws"):
        error("AWS CLI not found, please install it: https://aws.amazon.com/cli/")
        raise Exit(code=1)
    # Ensure azure cli is installed
    if not shutil.which("az"):
        error("Azure CLI not found, please install it: https://learn.microsoft.com/en-us/cli/azure/install-azure-cli")
        raise Exit(code=1)
    # Ensure gcloud cli is installed
    if not shutil.which("gcloud"):
        error("Gcloud CLI not found, please install it: https://cloud.google.com/sdk/docs/install")
        raise Exit(code=1)

    # Ensure gke-gcloud-auth-plugin is installed
    install_gcloud_auth_plugin(ctx)

    pulumi_version, pulumi_up_to_date = pulumi_version(ctx)
    if pulumi_up_to_date:
        info(f"Pulumi is up to date: {pulumi_version}")
    else:
        install_pulumi(ctx)

    with ctx.cd(get_pulumi_run_folder()):
        # install plugins
        ctx.run("pulumi --non-interactive plugin install")
        # login to local stack storage
        ctx.run("pulumi login --local")

    try:
        cfg = config.get_local_config(config_path)
    except Exception:
        cfg = config.Config.model_validate({})

    if interactive:
        info("🤖 Let's configure your environment for e2e tests! Press ctrl+c to stop me")
        # AWS config
        setup_aws_config(ctx, cfg)
        # Azure config
        setup_azure_config(cfg)
        # Gcp config
        setup_gcp_config(cfg)
        # Agent config
        setup_agent_config(cfg)
        # Pulumi config
        setup_pulumi_config(cfg)

        cfg.save_to_local_config(config_path)

    check_config(cfg)

    if debug:
        debug_env(ctx, config_path=config_path)

    if interactive:
        import pyperclip

        cat_profile_command = f"cat {config.get_full_profile_path(config_path)}"
        pyperclip.copy(cat_profile_command)
        print(
            f"\nYou can run the following command to print your configuration: `{cat_profile_command}`. This command was copied to the clipboard\n"
        )


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
    if config.configParams is None:
        error("configParams missing from config")
        raise Exit(code=1)
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

    # check .aws/config exists and contains expected profile
    # some invoke taskes hard code this value.
    expected_profile = 'sso-agent-sandbox-account-admin'
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

    # Check aws-vault profile name, some invoke taskes hard code this value.
    expected_profile = 'sso-agent-sandbox-account-admin'
    out = ctx.run("aws-vault list", hide=True)
    if expected_profile not in out.stdout:
        warn(f"WARNING: expected profile {expected_profile} missing from aws-vault. Some invoke tasks may fail.")
        print()

    debug_keys(ctx, config_path=config_path)
