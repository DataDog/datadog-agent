from pathlib import Path
from typing import Optional

import yaml
from invoke.exceptions import Exit
from pydantic import BaseModel, ConfigDict
from termcolor import colored

from .tool import info

profile_filename = ".test_infra_config.yaml"


class Config(BaseModel):
    model_config = ConfigDict(extra="forbid")  # noqa: vulture thinks it is unused

    class Params(BaseModel):
        model_config = ConfigDict(extra="forbid")  # noqa: vulture thinks it is unused

        class Aws(BaseModel):
            model_config = ConfigDict(extra="forbid")  # noqa: vulture thinks it is unused
            keyPairName: str | None
            publicKeyPath: str | None
            privateKeyPath: str | None = None
            privateKeyPassword: str | None = None
            account: str | None
            teamTag: str | None

            def get_account(self) -> str:
                if self.account is None:
                    return "agent-sandbox"
                if self.account == "sandbox":
                    print(
                        colored(
                            """
Warning: You are deploying to the sandbox account, this AWS account is no longer supported.
You should consider moving to the agent-sandbox account. Please follow https://datadoghq.atlassian.net/wiki/spaces/ADX/pages/3492282517/Getting+started+with+E2E to set it up.
                          """,
                            "yellow",
                        )
                    )
                return self.account

        aws: Aws | None

        class Azure(BaseModel):
            model_config = ConfigDict(extra="forbid")  # noqa: vulture thinks it is unused
            _DEFAULT_ACCOUNT = "agent-sandbox"
            publicKeyPath: str | None = None
            account: str | None = _DEFAULT_ACCOUNT

        azure: Azure | None = None

        class GCP(BaseModel):
            model_config = ConfigDict(extra="forbid")  # noqa: vulture thinks it is unused
            _DEFAULT_ACCOUNT = "agent-sandbox"
            publicKeyPath: str | None = None
            pullSecretPath: str | None = None
            account: str | None = _DEFAULT_ACCOUNT

        gcp: GCP | None = None

        class Local(BaseModel):
            model_config = ConfigDict(extra="forbid")  # noqa: vulture thinks it is unused
            publicKeyPath: str | None = None

        local: Local | None = None

        class Agent(BaseModel):
            model_config = ConfigDict(extra="forbid")  # noqa: vulture thinks it is unused
            apiKey: str | None
            appKey: str | None
            verifyCodeSignature: Optional[bool] = True  # noqa used in e2e tests

        agent: Agent | None

        class Pulumi(BaseModel):
            model_config = ConfigDict(extra="forbid")  # noqa: vulture thinks it is unused
            logLevel: int | None = None
            logToStdErr: bool | None = None
            verboseProgressStreams: Optional[bool] = None  # noqa used in e2e tests

        pulumi: Pulumi | None = None

        devMode: Optional[bool] = False  # noqa used in e2e tests

    configParams: Params | None = None

    stackParams: dict[str, dict[str, str]] | None = None

    class Options(BaseModel):
        model_config = ConfigDict(extra="forbid")  # noqa: vulture thinks it is unused
        checkKeyPair: bool | None

    options: Options | None = None

    def get_options(self) -> Options:
        if self.options is None:
            return Config.Options(checkKeyPair=False)
        return self.options

    def get_azure(self) -> Params.Azure:
        default = Config.Params.Azure(publicKeyPath=None)
        if self.configParams is None:
            return default
        if self.configParams.azure is None:
            return default
        return self.configParams.azure

    def get_gcp(self) -> Params.GCP:
        default = Config.Params.GCP(publicKeyPath=None)
        if self.configParams is None:
            return default
        if self.configParams.gcp is None:
            return default
        return self.configParams.gcp

    def get_aws(self) -> Params.Aws:
        default = Config.Params.Aws(keyPairName=None, publicKeyPath=None, account=None, teamTag=None)
        if self.configParams is None:
            return default
        if self.configParams.aws is None:
            return default
        return self.configParams.aws

    def get_local(self) -> Params.Local:
        default = Config.Params.Local(publicKeyPath=None)
        if self.configParams is None:
            return default
        if self.configParams.local is None:
            return default
        return self.configParams.local

    def get_agent(self) -> Params.Agent:
        default = Config.Params.Agent(apiKey=None, appKey=None)
        if self.configParams is None:
            return default
        if self.configParams.agent is None:
            return default
        return self.configParams.agent

    def get_pulumi(self) -> Params.Pulumi:
        default = Config.Params.Pulumi(
            logLevel=None,
            logToStdErr=None,
            verboseProgressStreams=None,
        )
        if self.configParams is None:
            return default
        if self.configParams.pulumi is None:
            return default
        return self.configParams.pulumi

    def get_stack_params(self) -> dict[str, dict[str, str]]:
        if self.stackParams is None:
            return {}
        return self.stackParams

    def save_to_local_config(self, config_path: str | None = None):
        profile_path = get_full_profile_path(config_path)
        try:
            with open(profile_path, "w") as outfile:
                yaml.dump(self.dict(), outfile)
        except Exception as e:
            raise Exit(f"Error saving config file {profile_path}: {e}") from e
        info(f"Configuration file saved at {profile_path}")


def get_local_config(profile_path: str | None = None) -> Config:
    profile_path = get_full_profile_path(profile_path)
    try:
        with open(profile_path) as f:
            content = f.read()
            config_dict = yaml.load(content, Loader=yaml.Loader)
            return Config.model_validate(config_dict)
    except FileNotFoundError:
        return Config.model_validate({})


def get_full_profile_path(profile_path: str | None = None) -> str:
    if profile_path:
        return str(
            Path(profile_path).expanduser().absolute()
        )  # Return absolute path to config file, handle "~"" with expanduser
    return str(Path.home().joinpath(profile_filename))
