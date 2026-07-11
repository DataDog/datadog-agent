import getpass
import json
import os
import secrets
from pathlib import Path

from invoke.context import Context
from invoke.exceptions import Exit, UnexpectedExit

from tasks.e2e_framework.config import Config
from tasks.e2e_framework.setup.ssh_keys import KeyInfo, add_key_to_ssh_agent, default_key_paths
from tasks.e2e_framework.tool import ask, ask_yesno, error, get_aws_cmd, info, is_windows, warn

SUPPORTED_KEY_TYPES = ["rsa", "ed25519"]
AVAILABLE_AWS_ACCOUNTS = ["agent-sandbox", "sandbox", "tse-playground"]
DEFAULT_AWS_ACCOUNT = "agent-sandbox"
DEFAULT_KEY_TYPE = "rsa"


def _default_keypair_name(account: str, user: str) -> str:
    return f"e2e-{account}-{user}".replace("_", "-")


def setup_aws_config(ctx: Context, config: Config, account: str | None = None):
    """
    Configure AWS keypair, SSO profile and team tag with computed defaults.

    Idempotent: re-running on a fully configured machine prints "✓ already configured"
    lines and exits without prompts. The only interactive step is the team tag, asked
    once on first setup.
    """
    if config.configParams.aws is None:
        config.configParams.aws = Config.Params.Aws(keyPairName=None, publicKeyPath=None, account=None, teamTag=None)

    aws = config.configParams.aws
    user = getpass.getuser()

    # Account
    if account:
        if account not in AVAILABLE_AWS_ACCOUNTS:
            raise Exit(f"Unknown AWS account: {account}. Available: {'|'.join(AVAILABLE_AWS_ACCOUNTS)}")
        aws.account = account
    elif not aws.account:
        aws.account = DEFAULT_AWS_ACCOUNT
    info(f"✓ AWS account: {aws.account}")

    # Keypair name & paths — derived from username, no prompt.
    if not aws.keyPairName:
        aws.keyPairName = _default_keypair_name(aws.account, user)
    default_priv, default_pub = default_key_paths(aws.account, user)
    if not aws.privateKeyPath:
        aws.privateKeyPath = str(default_priv)
    if not aws.publicKeyPath:
        aws.publicKeyPath = str(default_pub)

    # AWS authentication (SSO profile in ~/.aws/config + active aws-vault session) is
    # handled outside of this task — by your org tooling or manually. The keypair check
    # below uses aws-vault and will surface any auth errors with the standard aws-vault
    # output if the session is not valid.
    _ensure_aws_keypair(ctx, config)

    # Team tag — single prompt, only on first setup.
    if not aws.teamTag:
        team = ask(
            "🔖 GitHub team (used to tag AWS resources, kebab-case e.g. agent-platform) " "[default: unspecified]: ",
            color="cyan",
        ).strip()
        aws.teamTag = team or "unspecified"
        if aws.teamTag == "unspecified":
            warn(
                "Team tag set to 'unspecified' — update aws.teamTag in ~/.test_infra_config.yaml later for cost attribution"
            )
    info(f"✓ Team tag: {aws.teamTag}")


def _ensure_aws_keypair(ctx: Context, config: Config) -> None:
    """
    Make the configured keypair exist both in AWS and on disk. Branches:
    - Both present → ✓ skip.
    - Local files only → import to AWS.
    - AWS keypair only → fail with actionable message (don't auto-overwrite).
    - Neither → create in AWS and save locally.
    """
    assert config.configParams.aws is not None
    aws = config.configParams.aws
    keypair_name = aws.keyPairName or ""
    private_path = Path(aws.privateKeyPath or "").expanduser()
    public_path = Path(aws.publicKeyPath or "").expanduser()
    aws_account = aws.account

    info(f"🔍 Checking AWS keypair '{keypair_name}' (this may prompt for aws-vault auth)...")
    aws_has_keypair = _aws_keypair_exists(ctx, keypair_name, aws_account)
    local_has_files = private_path.is_file() and public_path.is_file()

    if aws_has_keypair and local_has_files:
        info(f"✓ AWS keypair '{keypair_name}' present on disk and in AWS")
        return

    if not aws_has_keypair and not local_has_files:
        info(f"🔑 Creating AWS keypair '{keypair_name}' → {private_path}")
        _aws_create_keypair(
            ctx,
            config,
            keypair_name=keypair_name,
            key_type=DEFAULT_KEY_TYPE,
            private_key_path=str(private_path),
            public_key_path=str(public_path),
            use_aws_vault=True,
            aws_account_name=aws_account,
        )
        return

    if local_has_files and not aws_has_keypair:
        info(f"🔑 Importing existing local key {public_path} as AWS keypair '{keypair_name}'")
        _aws_import_keypair(
            ctx,
            config,
            keypair_name=keypair_name,
            private_key_path=str(private_path),
            public_key_path=str(public_path),
            use_aws_vault=True,
            aws_account_name=aws_account,
        )
        return

    # AWS has the keypair, but local files are missing — don't auto-clobber.
    delete_cmd = get_aws_cmd(
        f'ec2 delete-key-pair --key-name "{keypair_name}"',
        use_aws_vault=True,
        aws_account=aws_account,
    )
    error(
        f"AWS already has keypair '{keypair_name}' but the local private/public key files "
        f"({private_path}, {public_path}) are missing. Either restore the files from a backup, "
        f"or delete the remote keypair and recreate with `dda inv e2e.setup.aws-create-keypair`:\n"
        f"  {delete_cmd}"
    )
    raise Exit(code=1)


def _aws_keypair_exists(ctx: Context, keypair_name: str, aws_account: str | None) -> bool:
    if not keypair_name:
        return False
    cmd = get_aws_cmd(
        f'ec2 describe-key-pairs --key-names "{keypair_name}"',
        use_aws_vault=True,
        aws_account=aws_account,
    )
    try:
        # Don't hide stdout/stderr: aws-vault may prompt for SSO auth or a keychain
        # password, and hiding the prompt makes the task look like it's hanging.
        # Suppress stdout via `out_stream` to /dev/null only after we know auth works
        # — but the simplest correct behavior is to leave the prompt visible.
        out = ctx.run(cmd, warn=True, hide="stdout")
    except UnexpectedExit:
        return False
    return out is not None and out.exited == 0


def setup_aws_sso_config(config: Config, interactive: bool = True):
    """
    Append the agent-sandbox SSO profile to ~/.aws/config if it isn't already there.

    When interactive=False (called from the wizard), no yes/no prompts are shown — the
    profile is added unconditionally. When interactive=True (used by the standalone
    e2e.setup.aws-sso task), the user is asked to confirm.
    """
    if not config.configParams.aws:
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
                info(f"✓ AWS SSO profile '{profile_name}' already in {aws_conf_path}")
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

    if interactive:
        info(conf)
        if not ask_yesno(f"Add the above config to {aws_conf_path}"):
            return
        if not ask_yesno(f"Do you want to setup AWS SSO profile for {aws.account}?"):
            return

    aws_conf_path.parent.mkdir(parents=True, exist_ok=True)
    with open(aws_conf_path, "a") as f:
        f.write(conf)
    info(f"✓ Wrote AWS SSO profile '{profile_name}' to {aws_conf_path}")


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
    if config.configParams.aws is None:
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

    # encrypt the private key with a random passphrase (matches token_urlsafe length used for Pulumi)
    passphrase = secrets.token_urlsafe(32)
    ctx.run(f'ssh-keygen -p -P "" -N "{passphrase}" -f "{private_key_path}"', hide=True)
    info("✓ Private key encrypted with passphrase (stored in ~/.test_infra_config.yaml, chmod 0600)")
    add_key_to_ssh_agent(ctx, private_key_path, passphrase)

    # update config object
    awsConf = config.configParams.aws
    if keypair_name:
        awsConf.keyPairName = keypair_name
    if public_key_path:
        awsConf.publicKeyPath = public_key_path
    if private_key_path:
        awsConf.privateKeyPath = private_key_path
    awsConf.privateKeyPassword = passphrase


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
    if config.configParams.aws is None:
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
    if config.configParams.aws is None:
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
    if config.configParams.aws is None:
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
