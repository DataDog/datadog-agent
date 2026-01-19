import base64
import getpass
import json
import os
import os.path
import shutil
from pathlib import Path
from typing import NamedTuple, Optional, Tuple

import pyperclip
from invoke.context import Context
from invoke.exceptions import Exit, UnexpectedExit
from invoke.tasks import task

from . import doc
from .config import Config, get_full_profile_path, get_local_config
from .tool import ask, ask_yesno, debug, error, get_aws_cmd, info, is_linux, is_windows, warn

available_aws_accounts = ["agent-sandbox", "sandbox", "agent-qa", "tse-playground"]
supported_key_types = ["rsa", "ed25519"]


@task(help={"config_path": doc.config_path, "interactive": doc.interactive, "debug": doc.debug}, default=True)
def setup(
    ctx: Context, config_path: Optional[str] = None, interactive: Optional[bool] = True, debug: Optional[bool] = False
) -> None:
    """
    Setup a local environment, interactively by default
    """
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
    _install_gcloud_auth_plugin(ctx)

    pulumi_version, pulumi_up_to_date = _pulumi_version(ctx)
    if pulumi_up_to_date:
        info(f"Pulumi is up to date: {pulumi_version}")
    else:
        _install_pulumi(ctx)

    # install plugins
    ctx.run("pulumi --non-interactive plugin install")
    # login to local stack storage
    ctx.run("pulumi login --local")

    try:
        config = get_local_config(config_path)
    except Exception:
        config = Config.model_validate({})

    if interactive:
        info("🤖 Let's configure your environment for e2e tests! Press ctrl+c to stop me")
        # AWS config
        setupAWSConfig(ctx, config)
        # Azure config
        setup_azure_config(config)
        # Gcp config
        setup_gcp_config(config)
        # Agent config
        setupAgentConfig(config)
        # Pulumi config
        setupPulumiConfig(config)

        config.save_to_local_config(config_path)

    _check_config(config)

    if debug:
        debug_env(ctx, config_path=config_path)

    if interactive:
        cat_profile_command = f"cat {get_full_profile_path(config_path)}"
        pyperclip.copy(cat_profile_command)
        print(
            f"\nYou can run the following command to print your configuration: `{cat_profile_command}`. This command was copied to the clipboard\n"
        )


def _install_pulumi(ctx: Context):
    info("🤖 Install Pulumi")
    if is_windows():
        ctx.run("winget install pulumi")
    elif is_linux():
        ctx.run("curl -fsSL https://get.pulumi.com | sh")
    else:
        ctx.run("brew install pulumi/tap/pulumi")
    # If pulumi was just installed for the first time it's probably not on the PATH,
    # add it to the process env so rest of setup can continue.
    if shutil.which("pulumi") is None:
        print()
        warn("Pulumi is not in the PATH, please add pulumi to PATH before running tests")
        if is_windows():
            # Add common pulumi install locations to PATH
            paths = [
                str(x)
                for x in [
                    Path().home().joinpath(".pulumi", "bin"),
                    Path().home().joinpath("AppData", "Local", "pulumi", "bin"),
                    'C:\\Program Files (x86)\\Pulumi\\bin',
                    'C:\\Program Files (x86)\\Pulumi',
                ]
            ]
            os.environ["PATH"] = ';'.join([os.environ["PATH"]] + paths)
        elif is_linux():
            path = Path().home().joinpath(".pulumi", "bin")
            os.environ["PATH"] = f"{os.environ['PATH']}:{path}"


# Check if gke-gcloud-auth-plugin is installed and install it if not
def _install_gcloud_auth_plugin(ctx):
    res = ctx.run('gcloud components list --format=json --filter "name: gke-gcloud-auth-plugin"', hide=True)
    installed_component = json.loads(res.stdout)
    if installed_component[0]["state"]["name"] == "Installed":
        print("✅ gke-gcloud-auth-plugin is already installed")
        return
    print("🤖 Installing gke-gcloud-auth-plugin")
    install = ctx.run("gcloud components install -q gke-gcloud-auth-plugin", hide=True)
    if install is None:
        raise Exit("Failed to install gke-gcloud-auth-plugin")
    if install.exited != 0:
        raise Exit(f"Failed to install gke-gcloud-auth-plugin: {install.stderr}")
    print("✅ gke-gcloud-auth-plugin installed")


def _check_config(config: Config):
    aws = config.get_aws()
    if aws.privateKeyPassword:
        warn("WARNING: privateKeyPassword is set. Please ensure privateKeyPath is used ONLY for E2E tests.")


def setupAWSConfig(ctx: Context, config: Config):
    if config.configParams is None:
        config.configParams = Config.Params(aws=None, agent=None, pulumi=None, azure=None, devMode=False)
    if config.configParams.aws is None:
        config.configParams.aws = Config.Params.Aws(keyPairName=None, publicKeyPath=None, account=None, teamTag=None)

    # aws account
    if config.configParams.aws.account is None:
        config.configParams.aws.account = "agent-sandbox"
    default_aws_account = config.configParams.aws.account
    while True:
        config.configParams.aws.account = default_aws_account
        aws_account = ask(
            f"Which aws account do you want to create instances on? Default [{config.configParams.aws.account}], available [agent-sandbox|sandbox|tse-playground]: "
        )
        if len(aws_account) > 0:
            config.configParams.aws.account = aws_account
        if config.configParams.aws.account in available_aws_accounts:
            break
        warn(f"{config.configParams.aws.account} is not a valid aws account")

    if config.configParams.aws.keyPairName and config.configParams.aws.publicKeyPath:
        info(f"Using key pair name: {config.configParams.aws.keyPairName}")
        info(f"Using public key path: {config.configParams.aws.publicKeyPath}")
        info(f"Using private key path: {config.configParams.aws.privateKeyPath}")

    # ask user if they want to create a new key or import an existing key
    if ask_yesno("Do you want to create a new key pair?"):
        _aws_create_keypair(ctx, config, use_aws_vault=True, aws_account_name=config.configParams.aws.account)
    elif ask_yesno("Do you want to import an existing key pair?"):
        _aws_import_keypair(ctx, config, use_aws_vault=True, aws_account_name=config.configParams.aws.account)

    if not config.configParams.aws.keyPairName or not config.configParams.aws.publicKeyPath:
        warn("No key pair configured, you will need to manually configure a key pair")

    # check keypair name
    if config.options is None:
        config.options = Config.Options(checkKeyPair=False)
    default_check_key_pair = "Y" if config.options.checkKeyPair else "N"
    checkKeyPair = ask(
        f"Do you want to check if the keypair is loaded in ssh agent when creating manual environments or running e2e tests [Y/N]? Default [{default_check_key_pair}]: "
    )
    if len(checkKeyPair) > 0:
        config.options.checkKeyPair = checkKeyPair.lower() == "y" or checkKeyPair.lower() == "yes"

    # team tag
    if config.configParams.aws.teamTag is None:
        config.configParams.aws.teamTag = ""
    while True:
        msg = "🔖 What is your github team? This will tag all your resources by `team:<team>`. Use kebab-case format (example: agent-platform)"
        if len(config.configParams.aws.teamTag) > 0:
            msg += f". Default [{config.configParams.aws.teamTag}]"
        msg += ": "
        teamTag = ask(msg)
        if len(teamTag) > 0:
            config.configParams.aws.teamTag = teamTag
        if len(config.configParams.aws.teamTag) > 0:
            break
        warn("Provide a non-empty team")

    _setup_aws_sso_config(config)


def _setup_aws_sso_config(config: Config):
    if not config.configParams or not config.configParams.aws:
        raise Exit("AWS config not found")

    aws = config.configParams.aws

    # agent-sandbox
    role = 'account-admin'
    acct_id = 376334461865
    start_url = 'https://d-906757b57c.awsapps.com/start/#'
    region = 'us-east-1'

    aws_conf_path = Path.home().joinpath(".aws", "config")
    profile_name = f'sso-{aws.account}-{role}'
    sso_session_name = profile_name

    # skip if profile already exists
    if os.path.isfile(aws_conf_path):
        with open(aws_conf_path) as f:
            conf = f.read()
            if profile_name in conf:
                info(f"Profile {profile_name} already exists in {aws_conf_path}")
                return

    if not ask_yesno(f"Do you want to setup AWS SSO profile for {aws.account}?"):
        return

    # https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-sso.html#cli-configure-sso-manual
    conf = f"""
# BEGIN Automatically added by e2e setup script

[profile {profile_name}]
sso_session = {sso_session_name}
sso_account_id = {acct_id}
sso_role_name = {role}
region = {region}

[sso-session {sso_session_name}]
sso_start_url = {start_url}
sso_region = {region}
sso_registration_scopes = sso:account:access

[profile exec-{profile_name}]
credential_process = aws-vault exec {profile_name} --json

# END Automatically added by e2e setup script
"""

    info(conf)
    if not ask_yesno(f"Add the above config to {aws_conf_path}"):
        return

    with open(aws_conf_path, "a") as f:
        f.write(conf)


@task
def aws_sso(ctx: Context, config_path: Optional[str] = None):
    """
    Setup AWS SSO profile for the agent-sandbox account if it doesn't exist

    Helper mainly here for Windows users who can't use the macos laptop setup script
    """
    try:
        config = get_local_config(config_path)
    except Exception as e:
        error(f"{e}")
        error("Failed to load config")
        raise Exit(code=1)

    _setup_aws_sso_config(config)


def setup_azure_config(config: Config):
    if config.configParams is None:
        config.configParams = Config.Params(aws=None, agent=None, pulumi=None, azure=None, gcp=None)
    if config.configParams.azure is None:
        config.configParams.azure = Config.Params.Azure(publicKeyPath=None)

    # azure public key path
    if config.configParams.azure.publicKeyPath is None:
        config.configParams.azure.publicKeyPath = str(Path.home().joinpath(".ssh", "id_ed25519.pub").absolute())
    default_public_key_path = config.configParams.azure.publicKeyPath
    while True:
        config.configParams.azure.publicKeyPath = default_public_key_path
        public_key_path = ask(
            f"🔑 Path to your Azure public ssh key: (default: [{config.configParams.azure.publicKeyPath}])"
        )
        if public_key_path:
            config.configParams.azure.publicKeyPath = public_key_path

        if os.path.isfile(config.configParams.azure.publicKeyPath):
            break
        warn(f"{config.configParams.azure.publicKeyPath} is not a valid ssh key")

    default_account = ask(f"🔑 Default account to use, default [{config.configParams.azure.account}]: ")
    if default_account:
        config.configParams.azure.account = default_account


def setup_gcp_config(config: Config):
    if config.configParams is None:
        config.configParams = Config.Params(aws=None, agent=None, pulumi=None, azure=None, gcp=None)
    if config.configParams.gcp is None:
        config.configParams.gcp = Config.Params.GCP(publicKeyPath=None)

    # gcp public key path
    if config.configParams.gcp.publicKeyPath is None:
        config.configParams.gcp.publicKeyPath = str(Path.home().joinpath(".ssh", "id_ed25519.pub").absolute())
    default_public_key_path = config.configParams.gcp.publicKeyPath
    while True:
        config.configParams.gcp.publicKeyPath = default_public_key_path
        public_key_path = ask(
            f"🔑 Path to your GCP public ssh key: (default: [{config.configParams.gcp.publicKeyPath}])"
        )
        if public_key_path:
            config.configParams.gcp.publicKeyPath = public_key_path

        if os.path.isfile(config.configParams.gcp.publicKeyPath):
            break
        warn(f"{config.configParams.gcp.publicKeyPath} is not a valid ssh key")

    default_account = ask(f"🔑 Default account to use, default [{config.configParams.gcp.account}]: ")
    if default_account:
        config.configParams.gcp.account = default_account

    # openShift pull secret path
    if config.configParams.gcp.pullSecretPath is None:
        config.configParams.gcp.pullSecretPath = ""
    default_pull_secret_path = config.configParams.gcp.pullSecretPath
    while True:
        config.configParams.gcp.pullSecretPath = default_pull_secret_path
        pull_secret_path = ask("🔑 Path to your OpenShift pull secret file (optional, can be set later): ")
        if not pull_secret_path:
            # empty to skip
            config.configParams.gcp.pullSecretPath = ""
            break

        config.configParams.gcp.pullSecretPath = pull_secret_path
        if os.path.isfile(config.configParams.gcp.pullSecretPath):
            break
        warn(f"{config.configParams.gcp.pullSecretPath} is not a valid file")


def setupAgentConfig(config):
    if config.configParams.agent is None:
        config.configParams.agent = Config.Params.Agent(
            apiKey=None,
            appKey=None,
        )
    # API key
    if config.configParams.agent.apiKey is None:
        config.configParams.agent.apiKey = "0" * 32
    default_api_key = config.configParams.agent.apiKey
    while True:
        config.configParams.agent.apiKey = default_api_key
        apiKey = ask(f"🐶 Datadog API key - default [{_get_safe_dd_key(config.configParams.agent.apiKey)}]: ")
        if len(apiKey) > 0:
            config.configParams.agent.apiKey = apiKey
        if len(config.configParams.agent.apiKey) == 32:
            break
        warn(f"Expecting API key of length 32, got {len(config.configParams.agent.apiKey)}")
    # APP key
    if config.configParams.agent.appKey is None:
        config.configParams.agent.appKey = "0" * 40
    default_app_key = config.configParams.agent.appKey
    while True:
        config.configParams.agent.appKey = default_app_key

        app_Key = ask(f"🐶 Datadog APP key - default [{_get_safe_dd_key(config.configParams.agent.appKey)}]: ")
        if len(app_Key) > 0:
            config.configParams.agent.appKey = app_Key
        if len(config.configParams.agent.appKey) == 40:
            break
        warn(f"Expecting APP key of length 40, got {len(config.configParams.agent.appKey)}")


def setupPulumiConfig(config):
    if config.configParams.pulumi is None:
        config.configParams.pulumi = Config.Params.Pulumi(
            logLevel=1,
            logToStdErr=False,
        )
    # log level
    if config.configParams.pulumi.logLevel is None:
        config.configParams.pulumi.logLevel = 1
    default_log_level = config.configParams.pulumi.logLevel
    info(
        "Pulumi emits logs at log levels between 1 and 11, with 11 being the most verbose. At log level 10 or below, Pulumi will avoid intentionally exposing any known credentials. At log level 11, Pulumi will intentionally expose some known credentials to aid with debugging, so these log levels should be used only when absolutely needed."
    )
    while True:
        log_level = ask(f"🔊 Pulumi log level (1-11) - empty defaults to [{default_log_level}]: ")
        if len(log_level) == 0:
            config.configParams.pulumi.logLevel = default_log_level
            break
        if log_level.isdigit() and 1 <= int(log_level) <= 11:
            config.configParams.pulumi.logLevel = int(log_level)
            break
        warn(f"Expecting log level between 1 and 11, got {log_level}")
    # APP key
    if config.configParams.pulumi.logToStdErr is None:
        config.configParams.pulumi.logToStdErr = False
    default_logs_to_std_err = config.configParams.pulumi.logToStdErr
    while True:
        logs_to_std_err = ask(f"Write pulumi logs to stderr - empty defaults to [{default_logs_to_std_err}]: ")
        if len(logs_to_std_err) == 0:
            config.configParams.pulumi.logToStdErr = default_logs_to_std_err
            break
        if logs_to_std_err.lower() in ["true", "false"]:
            config.configParams.pulumi.logToStdErr = logs_to_std_err.lower() == "true"
            break
        warn(f"Expecting one of [true, false], got {logs_to_std_err}")


def resolve_keypair_opts(
    config: Config,
    keypair_name: Optional[str] = None,
    key_type: Optional[str] = None,
    key_format: Optional[str] = None,
    private_key_path: Optional[Path | str] = None,
    public_key_path: Optional[Path | str] = None,
    require_key_type: Optional[bool] = False,
    require_keyfile_exists: Optional[bool] = False,
):
    """
    Resolve key pair options, in decreasing order of priority:
    - user input (parameters)
    - local config
    - defaults

    Returns a dict with the resolved values.
    """
    if config.configParams is None or config.configParams.aws is None:
        raise Exit("Config is missing aws section")
    awsConf = config.configParams.aws

    # use local config values as defaults if they are set
    if awsConf.keyPairName:
        default_keypair_name = awsConf.keyPairName
    else:
        default_keypair_name = getpass.getuser()
    if awsConf.privateKeyPath:
        default_private_key_path = awsConf.privateKeyPath
    else:
        default_private_key_path = None
    if awsConf.publicKeyPath:
        default_public_key_path = awsConf.publicKeyPath
    else:
        default_public_key_path = None

    # ask for missing values
    if not keypair_name:
        keypair_name = ask(f"🔑 Key pair name (default: {default_keypair_name}): ")
        if not keypair_name:
            keypair_name = default_keypair_name
    if not key_type and require_key_type:
        warn('Creating Windows VMs requires "rsa" key type')
        keytype_default = "rsa"
        while True:
            supported = "|".join(supported_key_types)
            key_type = ask(f"🔑 Key type [{supported}] (default: {keytype_default}): ")
            if not key_type:
                key_type = keytype_default
            if key_type in supported_key_types:
                break
            warn(f"{key_type} is not a supported key type")
    if not key_format:
        # Just use pem for now
        key_format = "pem"
    if not private_key_path:
        if default_private_key_path is None or keypair_name != default_keypair_name:
            # Example: ~/.ssh/id_rsa_e2e_agent_sandbox_mykeyname.pem
            account_part = f"{awsConf.account}_" if awsConf.account else ""
            account_part = account_part.replace("-", "_")
            default_private_key_path = Path.home().joinpath(
                ".ssh", f'id_{key_type or "rsa"}_e2e_{account_part}{keypair_name}.{key_format}'
            )
        while True:
            private_key_path = ask(f"🔑 Private key path (default: {default_private_key_path}): ")
            if not private_key_path:
                private_key_path = default_private_key_path
            if not require_keyfile_exists or os.path.isfile(private_key_path):
                break
            warn(f"{private_key_path} is not a valid ssh key")
    if not public_key_path:
        if default_public_key_path is None or keypair_name != default_keypair_name:
            filename_nosuffix = Path(private_key_path).stem
            parent_dir = Path(private_key_path).parent
            default_public_key_path = parent_dir.joinpath(f'{filename_nosuffix}.pub')
        while True:
            public_key_path = ask(f"🔑 Public key path (default: {default_public_key_path}): ")
            if not public_key_path:
                public_key_path = default_public_key_path
            if not require_keyfile_exists or os.path.isfile(public_key_path):
                break
            warn(f"{public_key_path} is not a valid ssh key")

    # pathlib expand paths
    private_key_path = Path(private_key_path).expanduser().resolve()
    public_key_path = Path(public_key_path).expanduser().resolve()

    return {
        "keypair_name": keypair_name,
        "key_type": key_type,
        "key_format": key_format,
        "private_key_path": private_key_path,
        "public_key_path": public_key_path,
    }


def update_config_aws_keypair(
    config: Config,
    config_path: Optional[str] = None,
) -> None:
    if config.configParams is None or config.configParams.aws is None:
        raise Exit("Config is missing aws section")
    awsConf = config.configParams.aws
    info(f"keyPairName: {awsConf.keyPairName}")
    info(f"publicKeyPath: {awsConf.publicKeyPath}")
    info(f"privateKeyPath: {awsConf.privateKeyPath}")
    if ask_yesno("Do you want to update the local config?"):
        config.save_to_local_config(config_path)


def check_existing_aws_keypair(
    ctx: Context,
    keypair_name: str,
    use_aws_vault: Optional[bool] = False,
    aws_account_name: Optional[str] = None,
) -> bool:
    """
    Check if aws key pair already exists.
    If it does, ask if user wants to overwrite it.
    If user does want to overwrite it, delete the existing key pair.

    Return True if key pair does not exist or user wants to overwrite it.
    """

    def _get_aws_cmd(cmd):
        return get_aws_cmd(cmd, use_aws_vault=use_aws_vault, aws_account=aws_account_name)

    # check if key pair already exists
    cmd = f'ec2 describe-key-pairs --key-names "{keypair_name}"'
    out = ctx.run(_get_aws_cmd(cmd), hide=True, warn=True)
    if out is None:
        raise Exit(f"Failed to check if key pair {keypair_name} exists")
    if out.exited == 0:
        warn(f"Key pair {keypair_name} already exists.")
        if not ask_yesno("Do you want to overwrite it?"):
            return False
        # delete existing key pair
        cmd = f'ec2 delete-key-pair --key-name "{keypair_name}"'
        ctx.run(_get_aws_cmd(cmd))
    return True


@task
def aws_create_keypair(
    ctx: Context,
    keypair_name: Optional[str] = None,
    key_type: Optional[str] = None,
    private_key_path: Optional[str] = None,
    public_key_path: Optional[str] = None,
    use_aws_vault: Optional[bool] = False,
    aws_account_name: Optional[str] = None,
    config_path: Optional[str] = None,
) -> None:
    """
    Create a new key pair using the AWS CLI, save the key pair to disk, and update the local config.

    This task is interactive and can be run with no options:
      - Prompt user for missing values
      - Confirm overwrite of existing key pair
      - Confirm update of local config
    """
    try:
        config = get_local_config(config_path)
    except Exception as e:
        error(f"{e}")
        error("Failed to load config")
        raise Exit(code=1)

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


def _aws_create_keypair(
    ctx: Context,
    config: Config,
    keypair_name: Optional[str] = None,
    key_type: Optional[str] = None,
    private_key_path: Optional[str] = None,
    public_key_path: Optional[str] = None,
    use_aws_vault: Optional[bool] = False,
    aws_account_name: Optional[str] = None,
) -> None:
    if config.configParams is None or config.configParams.aws is None:
        raise Exit("Config is missing aws section")

    keypair_opts = resolve_keypair_opts(
        config=config,
        keypair_name=keypair_name,
        key_type=key_type,
        private_key_path=private_key_path,
        public_key_path=public_key_path,
        require_key_type=True,
        require_keyfile_exists=False,
    )
    keypair_name = str(keypair_opts["keypair_name"])
    key_type = str(keypair_opts["key_type"])
    private_key_path = str(keypair_opts["private_key_path"])
    public_key_path = str(keypair_opts["public_key_path"])

    def _get_aws_cmd(cmd):
        return get_aws_cmd(cmd, use_aws_vault=use_aws_vault, aws_account=aws_account_name)

    # check if key pair already exists
    if not check_existing_aws_keypair(
        ctx, keypair_name, use_aws_vault=use_aws_vault, aws_account_name=aws_account_name
    ):
        return
    if Path(private_key_path).exists():
        warn(f"Private key {private_key_path} already exists.")
        if not ask_yesno("Do you want to overwrite it?"):
            return
        # delete existing key pair
        os.remove(private_key_path)

    # generate private key
    cmd = f'ec2 create-key-pair --key-name "{keypair_name}" --key-type {key_type} --query KeyMaterial --output text'
    out = ctx.run(_get_aws_cmd(cmd), hide=True)
    if out is None:
        raise Exit(f"Failed to create key pair {keypair_name}")
    key_material = out.stdout.strip()
    # write private key to disk
    os.makedirs(Path(private_key_path).parent, exist_ok=True)
    with open(private_key_path, "w") as f:
        f.write(key_material)
    if not is_windows():
        os.chmod(private_key_path, 0o600)
        # Windows permissions should be fine as is via inheritance

    # generate public key from private key
    cmd = f'ssh-keygen -f "{private_key_path}" -y'
    out = ctx.run(cmd, hide=True)
    if out is None:
        raise Exit(f"Failed to generate public key from private key {private_key_path}")
    public_key = out.stdout.strip()
    # write public key to disk
    with open(public_key_path, "w") as f:
        f.write(public_key)

    # update config object
    awsConf = config.configParams.aws
    if keypair_name:
        awsConf.keyPairName = keypair_name
    if public_key_path:
        awsConf.publicKeyPath = public_key_path
    if private_key_path:
        awsConf.privateKeyPath = private_key_path


@task
def aws_import_keypair(
    ctx: Context,
    keypair_name: Optional[str] = None,
    private_key_path: Optional[str] = None,
    public_key_path: Optional[str] = None,
    use_aws_vault: Optional[bool] = False,
    aws_account_name: Optional[str] = None,
    config_path: Optional[str] = None,
) -> None:
    """
    Import an existing key pair to AWS and update the config.

    This task is interactive and can be run with no options:
      - Prompt user for missing values
      - Confirm overwrite of existing key pair
      - Confirm update of local config
    """
    try:
        config = get_local_config(config_path)
    except Exception as e:
        error(f"{e}")
        error("Failed to load config")
        raise Exit(code=1)

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


def _aws_import_keypair(
    ctx: Context,
    config: Config,
    keypair_name: Optional[str] = None,
    private_key_path: Optional[str] = None,
    public_key_path: Optional[str] = None,
    use_aws_vault: Optional[bool] = False,
    aws_account_name: Optional[str] = None,
) -> None:
    if config.configParams is None or config.configParams.aws is None:
        raise Exit("Config is missing aws section")

    keypair_opts = resolve_keypair_opts(
        config=config,
        keypair_name=keypair_name,
        private_key_path=private_key_path,
        public_key_path=public_key_path,
        require_key_type=False,
        require_keyfile_exists=True,
    )
    keypair_name = str(keypair_opts["keypair_name"])
    private_key_path = str(keypair_opts["private_key_path"])
    public_key_path = str(keypair_opts["public_key_path"])

    def _get_aws_cmd(cmd):
        return get_aws_cmd(cmd, use_aws_vault=use_aws_vault, aws_account=aws_account_name)

    # check if key pair already exists
    if not check_existing_aws_keypair(
        ctx, keypair_name, use_aws_vault=use_aws_vault, aws_account_name=aws_account_name
    ):
        return

    # upload public key to aws
    cmd = f'ec2 import-key-pair --key-name "{keypair_name}" --public-key-material "fileb://{public_key_path}"'
    ctx.run(_get_aws_cmd(cmd))
    info(f"Public key imported to AWS as key pair {keypair_name}")

    # update config object
    awsConf = config.configParams.aws
    if keypair_name:
        awsConf.keyPairName = keypair_name
    if public_key_path:
        awsConf.publicKeyPath = public_key_path
    if private_key_path:
        awsConf.privateKeyPath = private_key_path


def _get_safe_dd_key(key: str) -> str:
    if key == "0" * len(key):
        return key
    return "*" * len(key)


def _pulumi_version(ctx: Context) -> Tuple[str, bool]:
    """
    Returns True if pulumi is installed and up to date, False otherwise
    Will return True if PULUMI_SKIP_UPDATE_CHECK=1
    """
    try:
        out = ctx.run("pulumi version --logtostderr", hide=True)
    except UnexpectedExit:
        # likely pulumi command not found
        return "", False
    if out is None:
        return "", False
    # The update message differs some between platforms so choose a common part
    up_to_date = "A new version of Pulumi is available" not in out.stderr
    return out.stdout.strip(), up_to_date


def ssh_fingerprint_to_bytes(fingerprint: str) -> bytes:
    out = fingerprint.strip().split(' ')[1]
    if out.count(':') > 1:
        # EXAMPLE: MD5(stdin)= 81:e4:46:e9:dd:a6:3d:41:6d:ca:94:21:5c:e5:1d:24
        # EXAMPLE: 2048 MD5:19:b3:a8:5f:13:7e:b9:d3:6c:75:20:d6:18:7f:e2:1d no comment (RSA)
        if out.startswith('MD5') or out.startswith('SHA'):
            out = out.split(':', 1)[1]
        return bytes.fromhex(out.replace(':', ''))
    else:
        # EXAMPLE: 256 SHA1:41jsg4Z9lgylj6/zmhGxtZ6/qZs testname (ED25519)
        # ssh leaves out padding but python will ignore extra padding so add the missing padding
        out = out.split(':', 1)
        return base64.b64decode(out[1] + '==')


# noqa: because vulture thinks this is unused
class KeyFingerprint(NamedTuple):
    md5: bytes  # noqa
    sha1: bytes  # noqa
    sha256: bytes  # noqa
    ssh_keygen: bytes  # noqa
    md5_import: bytes  # noqa


class KeyInfo(NamedTuple('KeyFingerprint', [('path', str), ('fingerprint', KeyFingerprint), ('is_rsa_pubkey', bool)])):
    def in_ssh_agent(self, ctx):
        out = ctx.run("ssh-add -l", hide=True)
        inAgent = out.stdout.strip().split('\n')
        for line in inAgent:
            line = line.strip()
            if not line:
                continue
            out = ssh_fingerprint_to_bytes(line)
            if self.match(out):
                return True
        return False

    def match(self, fingerprint: bytes):
        for f in self.fingerprint:
            if f == fingerprint:
                return True
        return False

    def match_ec2_keypair(self, keypair):
        # EC2 uses a different fingerprint hash/format depending on the key type and the key's origin
        # https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/verify-keys.html
        ec2_fingerprint = keypair["KeyFingerprint"]
        if ':' in ec2_fingerprint:
            ec2_fingerprint = bytes.fromhex(ec2_fingerprint.replace(':', ''))
        else:
            ec2_fingerprint = base64.b64decode(ec2_fingerprint + '==')
        return self.match(ec2_fingerprint)

    @classmethod
    def from_path(cls, ctx, path):
        fingerprints = {'ssh_keygen': b'', 'md5_import': b''}
        is_rsa_pubkey = False
        with open(path, 'rb') as f:
            firstline = f.readline()
            # Make sure the key is ascii
            if b'\0' in firstline:
                raise ValueError(f"Key file {path} is not ascii, it may be in utf-16, please convert it to ascii")
            if firstline.startswith(b'ssh-rsa'):
                is_rsa_pubkey = True
            # EC2 uses a different fingerprint hash/format depending on the key type and the key's origin
            # https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/verify-keys.html
            if b'SSH' in firstline or firstline.startswith(b'ssh-'):

                def getfingerprint(fmt, path):
                    out = ctx.run(f"ssh-keygen -l -E {fmt} -f \"{path}\"", hide=True)
                    return ssh_fingerprint_to_bytes(out.stdout.strip())

            elif b'BEGIN' in firstline:

                def getfingerprint(fmt, path):
                    out = ctx.run(
                        f'openssl pkcs8 -in "{path}" -inform PEM -outform DER -topk8 -nocrypt | openssl {fmt} -c',
                        hide=True,
                    )
                    # EXAMPLE: (stdin)= e3:a8:bc:0a:3a:54:9f:b8:be:6e:75:8c:98:26:8e:3d:8e:e9:d0:69
                    out = out.stdout.strip().split(' ')[1]
                    return bytes.fromhex(out.replace(':', ''))

                # AWS calculatees its fingerprints differents for RSA keys,
                # such that the sha256 fingerprint doesn't match ssh-agent/ssh-keygen.
                # It seems like they're hashing the private key instead of the public key.
                # This also means it's not possible to match a public key to an EC2 RSA fingerprint
                # if AWS generated the private key.
                out = ctx.run(f"ssh-keygen -l -f {path}", hide=True)
                fingerprints['ssh_keygen'] = ssh_fingerprint_to_bytes(out.stdout.strip())
                # If the key was imported to AWS, the fingerprint is calculated off the public key data
                out = ctx.run(
                    f"ssh-keygen -ef {path} -m PEM | openssl rsa -RSAPublicKey_in -outform DER | openssl md5 -c",
                    hide=True,
                )
                fingerprints['md5_import'] = ssh_fingerprint_to_bytes(out.stdout.strip())
            else:
                raise ValueError(f"Key file {path} is not a valid ssh key")
        # aws returns fingerprints in different formats so get a couple
        for fmt in ['md5', 'sha1', 'sha256']:
            fingerprints[fmt] = getfingerprint(fmt, path)
        return cls(path=path, fingerprint=KeyFingerprint(**fingerprints), is_rsa_pubkey=is_rsa_pubkey)


def load_ec2_keypairs(ctx: Context) -> dict:
    out = ctx.run("aws ec2 describe-key-pairs --output json", hide=True)
    if not out or out.exited != 0:
        warn("No AWS keypair found, please create one")
        return {}
    jso = json.loads(out.stdout)
    keypairs = jso.get("KeyPairs", None)
    if keypairs is None:
        warn("No AWS keypair found, please create one")
        return {}
    return keypairs


def find_matching_ec2_keypair(ctx: Context, keypairs: dict, path: Path) -> Tuple[Optional[KeyInfo], Optional[dict]]:
    if not os.path.exists(path):
        warn(f"WARNING: Key file {path} does not exist")
        return None, None
    info = KeyInfo.from_path(ctx, path)
    for keypair in keypairs:
        if info.match_ec2_keypair(keypair):
            return info, keypair
    return info, None


def get_ssh_keys():
    ignore = ["known_hosts", "authorized_keys", "config"]
    root = Path.home().joinpath(".ssh")
    filenames = filter(lambda x: x.is_file() and x not in ignore, root.iterdir())
    return list(map(root.joinpath, filenames))


def _check_key(ctx: Context, keyinfo: KeyInfo, keypair: dict, configuredKeyPairName: str):
    if keypair["KeyName"] != configuredKeyPairName:
        warn("WARNING: Key name does not match configured keypair name. This key will not be used for provisioning.")
    if _ssh_agent_supported():
        if not keyinfo.in_ssh_agent(ctx):
            warn("WARNING: Key missing from ssh-agent. This key will not be used for connections.")
    if "rsa" not in keypair["KeyType"].lower():
        warn("WARNING: Key type is not RSA. This key cannot be used to decrypt Windows RDP credentials.")


def _passphrase_decrypts_privatekey(ctx: Context, path: str, passphrase: str):
    try:
        ctx.run(f"ssh-keygen -y -P '{passphrase}' -f {path}", hide=True)
    except UnexpectedExit as e:
        # incorrect passphrase supplied to decrypt private key
        if 'incorrect passphrase' in str(e):
            return False
    return True


def _is_key_encrypted(ctx: Context, path: str):
    return not _passphrase_decrypts_privatekey(ctx, path, "")


def _ssh_agent_supported():
    return not is_windows()


@task(help={"config_path": doc.config_path})
def debug_keys(ctx: Context, config_path: Optional[str] = None):
    """
    Debug E2E and test-infra-definitions SSH keys
    """
    if _ssh_agent_supported():
        # Ensure ssh-agent is running
        try:
            ctx.run("ssh-add -l", hide=True)
        except UnexpectedExit as e:
            error(f"{e}")
            error("ssh-agent not available or no keys are loaded, please start it and load your keys")
            raise Exit(code=1)

    found = False
    keypairs = load_ec2_keypairs(ctx)

    info("Checking for valid SSH key configuration")

    # Get keypair name
    try:
        config = get_local_config(config_path)
    except Exception as e:
        error(f"{e}")
        error("Failed to load config")
        raise Exit(code=1)
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
    if awsConf.privateKeyPath and _is_key_encrypted(ctx, awsConf.privateKeyPath):
        if awsConf.privateKeyPassword:
            if not _passphrase_decrypts_privatekey(ctx, awsConf.privateKeyPath, awsConf.privateKeyPassword):
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
            _check_key(ctx, keyinfo, keypair, keypair_name)
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
            _check_key(ctx, keyinfo, keypair, keypair_name)
            print()
            found = True

    if not found:
        error("No matching keypair found in aws!")
        info(
            "If this is unexpected, confirm that your aws credential's region matches the region you uploaded your key to."
        )
        raise Exit(code=1)


@task(name="debug", help={"config_path": doc.config_path})
def debug_env(ctx, config_path: Optional[str] = None):
    """
    Debug E2E and test-infra-definitions required tools and configuration
    """
    # check pulumi found
    try:
        out = ctx.run("pulumi version", hide=True)
    except UnexpectedExit as e:
        error(f"{e}")
        error("Pulumi CLI not found, please install it: https://www.pulumi.com/docs/get-started/install/")
        raise Exit(code=1)
    info(f"Pulumi version: {out.stdout.strip()}")

    # Check pulumi credentials
    try:
        out = ctx.run("pulumi whoami", hide=True)
    except UnexpectedExit:
        error("No pulumi credentials found")
        info("Please login, e.g. pulumi login --local")
        raise Exit(code=1)

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
        raise Exit(code=1)
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
        raise Exit(code=1)

    print()

    # Check aws-vault profile name, some invoke taskes hard code this value.
    expected_profile = 'sso-agent-sandbox-account-admin'
    out = ctx.run("aws-vault list", hide=True)
    if expected_profile not in out.stdout:
        warn(f"WARNING: expected profile {expected_profile} missing from aws-vault. Some invoke tasks may fail.")
        print()

    debug_keys(ctx, config_path=config_path)
