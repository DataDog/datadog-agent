import getpass
import json
import os
from pathlib import Path

from invoke.context import Context
from invoke.exceptions import Exit

from tasks.e2e_framework.config import Config
from tasks.e2e_framework.setup.ssh_keys import KeyInfo
from tasks.e2e_framework.tool import ask, ask_yesno, get_aws_cmd, info, is_windows, warn

SUPPORTED_KEY_TYPES = ["rsa", "ed25519"]
AVAILABLE_AWS_ACCOUNTS = ["agent-sandbox", "sandbox", "tse-playground"]


def setup_aws_config(ctx: Context, config: Config):
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
        if config.configParams.aws.account in AVAILABLE_AWS_ACCOUNTS:
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

    setup_aws_sso_config(config)


def setup_aws_sso_config(config: Config):
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


def _aws_create_keypair(
    ctx: Context,
    config: Config,
    keypair_name: str | None = None,
    key_type: str | None = None,
    private_key_path: str | None = None,
    public_key_path: str | None = None,
    use_aws_vault: bool | None = False,
    aws_account_name: str | None = None,
) -> None:
    if config.configParams is None or config.configParams.aws is None:
        raise Exit("Config is missing aws section")

    keypair_opts = aws_resolve_keypair_opts(
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


def aws_resolve_keypair_opts(
    config: Config,
    keypair_name: str | None = None,
    key_type: str | None = None,
    key_format: str | None = None,
    private_key_path: Path | str | None = None,
    public_key_path: Path | str | None = None,
    require_key_type: bool | None = False,
    require_keyfile_exists: bool | None = False,
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
            supported = "|".join(SUPPORTED_KEY_TYPES)
            key_type = ask(f"🔑 Key type [{supported}] (default: {keytype_default}): ")
            if not key_type:
                key_type = keytype_default
            if key_type in SUPPORTED_KEY_TYPES:
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


def check_existing_aws_keypair(
    ctx: Context,
    keypair_name: str,
    use_aws_vault: bool | None = False,
    aws_account_name: str | None = None,
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


def update_config_aws_keypair(
    config: Config,
    config_path: str | None = None,
) -> None:
    if config.configParams is None or config.configParams.aws is None:
        raise Exit("Config is missing aws section")
    awsConf = config.configParams.aws
    info(f"keyPairName: {awsConf.keyPairName}")
    info(f"publicKeyPath: {awsConf.publicKeyPath}")
    info(f"privateKeyPath: {awsConf.privateKeyPath}")
    if ask_yesno("Do you want to update the local config?"):
        config.save_to_local_config(config_path)


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


def find_matching_ec2_keypair(ctx: Context, keypairs: dict, path: Path) -> tuple[KeyInfo | None, dict | None]:
    if not os.path.exists(path):
        warn(f"WARNING: Key file {path} does not exist")
        return None, None
    info = KeyInfo.from_path(ctx, path)
    for keypair in keypairs:
        if info.match_ec2_keypair(keypair):
            return info, keypair
    return info, None


def _aws_import_keypair(
    ctx: Context,
    config: Config,
    keypair_name: str | None = None,
    private_key_path: str | None = None,
    public_key_path: str | None = None,
    use_aws_vault: bool | None = False,
    aws_account_name: str | None = None,
) -> None:
    if config.configParams is None or config.configParams.aws is None:
        raise Exit("Config is missing aws section")

    keypair_opts = aws_resolve_keypair_opts(
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
