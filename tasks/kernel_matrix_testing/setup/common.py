import getpass
import importlib
import os
import re
import shutil
from datetime import timedelta
from pathlib import Path

from invoke.context import Context

from tasks.libs.common.status import Status
from tasks.libs.common.utils import get_repo_root, is_installed
from tasks.libs.types.arch import Arch

from ..compiler import CompilerImage
from ..kmt_os import get_kmt_os
from ..vars import AWS_ACCOUNT
from .requirement import Requirement, RequirementState
from .utils import check_directories


def get_requirements() -> list[Requirement]:
    return [
        Pulumi(),
        TestInfraDefinitionsRepo(),
        PulumiPlugin(),
        DDA(),
        PythonDependencies(),
        KMTDirectoriesRequirement(),
        KMTSSHKey(),
        Compiler(),
        Docker(),
        AWSConfig(),
    ]


class Pulumi(Requirement):
    def _check_user_logged_in(self, ctx: Context) -> RequirementState:
        pulumi_about_res = ctx.run("pulumi about", warn=True)
        if pulumi_about_res is None or not pulumi_about_res.ok:
            return RequirementState(Status.FAIL, "pulumi is not installed correctly.")

        match = re.search(r"^User +(.*)", pulumi_about_res.stdout, re.MULTILINE)
        if match is not None and match.group(1) != "Unknown":
            return RequirementState(Status.OK, "pulumi is installed and logged in.")

        return RequirementState(Status.FAIL, "pulumi is not logged in.")

    def check(self, ctx: Context, fix: bool) -> RequirementState:
        if shutil.which("pulumi") is None:
            if Path("~/.pulumi/bin/pulumi").expanduser().exists():
                return RequirementState(
                    Status.FAIL,
                    "pulumi installed in ~/.pulumi/bin but not in $PATH. Please add that path to your shell configuration",
                )

            if not fix:
                return RequirementState(Status.FAIL, "pulumi is not installed.", fixable=True)

            res = ctx.run("curl -fsSL https://get.pulumi.com | sh", warn=True)
            if res is None or not res.ok:
                return RequirementState(Status.FAIL, "pulumi installation failed.")

            # Update PATH in this session so we can call the command
            os.environ["PATH"] = f"{os.environ['PATH']}:{os.path.expanduser('~/.pulumi/bin')}"

        login_state = self._check_user_logged_in(ctx)
        if login_state.state != Status.OK:
            if not fix:
                return login_state

            try:
                ctx.run("pulumi login --local")
            except Exception as e:
                return RequirementState(Status.FAIL, f"pulumi login command failed: {e}")

            login_state = self._check_user_logged_in(ctx)

        return login_state


class TestInfraDefinitionsRepo(Requirement):
    @staticmethod
    def get_repo_path() -> Path:
        return get_repo_root() / "test/e2e-framework"

    def check(self, ctx: Context, fix: bool) -> RequirementState:
        repo_path = self.get_repo_path()
        if repo_path.is_dir():
            return RequirementState(Status.OK, "e2e-framework directory found.")

        return RequirementState(
            Status.FAIL,
            f"e2e-framework directory not found at {repo_path}.",
            fixable=False,
        )


class PulumiPlugin(Requirement):
    dependencies: list[type[Requirement]] = [Pulumi, TestInfraDefinitionsRepo]

    def check(self, ctx: Context, fix: bool) -> RequirementState:
        test_infra_repo_run = os.path.join(TestInfraDefinitionsRepo.get_repo_path(), "run")

        with ctx.cd(test_infra_repo_run):
            res = ctx.run("pulumi --non-interactive plugin ls", warn=True)
            # If there are more than 3 lines, then there are plugins installed. The other lines are headers/footers.
            if res is not None and res.ok and len(res.stdout.splitlines()) > 3:
                return RequirementState(Status.OK, "pulumi plugins installed.")

            if not fix:
                return RequirementState(Status.FAIL, "pulumi plugins not installed.", fixable=True)

            try:
                # https://github.com/golang/go/issues/63758: downloading dependencies stuck when git tag signature [...]
                ctx.run("GIT_TERMINAL_PROMPT=0 go mod download", timeout=timedelta(minutes=5).total_seconds())
                ctx.run("PULUMI_CONFIG_PASSPHRASE=dummy pulumi --non-interactive plugin install")
            except Exception as e:
                return RequirementState(Status.FAIL, f"pulumi plugins installation failed: {e}")

        return RequirementState(Status.OK, "pulumi plugins installed.")


class DDA(Requirement):
    def check(self, ctx: Context, fix: bool) -> RequirementState:
        import semver

        if not is_installed("dda"):
            return RequirementState(
                Status.FAIL,
                "dda is not installed, check https://datadoghq.dev/datadog-agent-dev/install/ for install instructions.",
            )

        dda_version = ctx.run("dda --version", warn=True)
        if dda_version is None or not dda_version.ok:
            return RequirementState(Status.FAIL, "dda is not installed correctly, cannot get version.")

        dda_version_file = get_repo_root() / ".dda/version"
        min_version = semver.VersionInfo.parse(dda_version_file.read_text().strip())
        version_parsed = semver.VersionInfo.parse(dda_version.stdout.strip().split(" ")[-1])

        if version_parsed < min_version:
            if not fix:
                return RequirementState(
                    Status.FAIL,
                    f"dda {version_parsed} is too old, please upgrade to at least {min_version}",
                    fixable=True,
                )

            try:
                ctx.run("dda self update")
            except Exception as e:
                return RequirementState(Status.FAIL, f"Failed to update dda: {e}")

        return RequirementState(Status.OK, f"dda is installed (version {dda_version.stdout.strip()} >= {min_version}).")


class PythonDependencies(Requirement):
    dependencies: list[type[Requirement]] = [DDA]

    # These are the packages that are required for KMT to work.
    # We cannot access the definition in dda's pyproject.toml, so we hardcode them
    # here, as they're only needed to ensure that the KMT group is installed,
    # so even if new packages are added to the group in dda code, the validation
    # will still work
    kmt_required_packages = ["termcolor", "thefuzz"]

    def check(self, ctx: Context, fix: bool) -> RequirementState:
        missing_dependencies = []
        for dep in self.kmt_required_packages:
            # Try importing the module dynamically, we don't really
            # care how it's installed, as long as it's installed.
            try:
                importlib.import_module(dep)
            except ImportError:
                missing_dependencies.append(dep)

        if len(missing_dependencies) == 0:
            return RequirementState(Status.OK, "Python dependencies are already installed.")

        if not fix:
            return RequirementState(
                Status.FAIL,
                f"Python dependencies not installed: {', '.join(missing_dependencies)}",
                fixable=True,
            )

        try:
            ctx.run("dda self dep sync -f legacy-kernel-matrix-testing")
            ctx.run("dda inv --feat legacy-kernel-matrix-testing -- --help", hide=True)
        except Exception as e:
            return RequirementState(Status.FAIL, f"Failed to install Python dependencies: {e}")

        return RequirementState(Status.OK, "Python dependencies installed.")

    def flare(self, ctx: Context) -> dict[str, str]:
        data: dict[str, str] = {}

        data["dda path"] = shutil.which('dda') or 'not found'

        dep_res = ctx.run("dda self dep show --legacy", warn=True)
        data["dda self dep show --legacy [stdout]"] = dep_res.stdout if dep_res is not None else "None"
        data["dda self dep show --legacy [stderr]"] = dep_res.stderr if dep_res is not None else "None"

        dda_version_res = ctx.run("dda --version", warn=True)
        data["dda version [stdout]"] = dda_version_res.stdout.strip() if dda_version_res is not None else "None"
        data["dda version [stderr]"] = dda_version_res.stderr if dda_version_res is not None else "None"

        return data


class KMTDirectoriesRequirement(Requirement):
    def check(self, ctx: Context, fix: bool) -> list[RequirementState]:
        kmt_os = get_kmt_os()
        dirs = [
            kmt_os.shared_dir,
            kmt_os.kmt_dir,
            kmt_os.packages_dir,
            kmt_os.stacks_dir,
            kmt_os.shared_dir,
        ]

        user = getpass.getuser()
        group = kmt_os.user_group

        return check_directories(ctx, dirs, fix, user, group, 0o755)


class KMTSSHKey(Requirement):
    dependencies: list[type[Requirement]] = [KMTDirectoriesRequirement]

    def check(self, ctx: Context, fix: bool) -> RequirementState:
        kmt_os = get_kmt_os()
        if not kmt_os.ddvm_rsa.exists():
            if not fix:
                return RequirementState(Status.FAIL, "KMT SSH key not found.", fixable=True)

            ddvm_rsa_key = get_repo_root() / "tasks" / "kernel_matrix_testing" / "ddvm_rsa"
            ctx.run(f"cp {ddvm_rsa_key} {kmt_os.kmt_dir}")

        perms = kmt_os.ddvm_rsa.stat().st_mode
        if (perms & 0o777) != 0o600:
            if not fix:
                return RequirementState(Status.FAIL, "KMT SSH key has incorrect permissions.", fixable=True)

            ctx.run(f"chmod 600 {kmt_os.kmt_dir}/ddvm_rsa")

        return RequirementState(Status.OK, "KMT SSH key created and with correct permissions.")


class Docker(Requirement):
    def check(self, ctx: Context, fix: bool) -> RequirementState:
        if shutil.which("docker") is None:
            return RequirementState(
                Status.FAIL,
                "docker is not installed, check https://docs.docker.com/engine/install/ for installation instructions.",
            )

        return RequirementState(Status.OK, "docker is installed.")


class Compiler(Requirement):
    dependencies: list[type[Requirement]] = [Docker]

    def check(self, ctx: Context, fix: bool) -> RequirementState:
        compiler = CompilerImage(ctx, Arch.local())

        state = self._check_compiler_state(compiler)

        if state.state == Status.OK or not fix or not state.fixable:
            return state

        # All the possible failure states have the same fix: start the compiler.
        compiler.start()
        return RequirementState(Status.OK, "Compiler started.")

    def _check_compiler_state(self, compiler: CompilerImage) -> RequirementState:
        if not compiler.is_loaded:
            return RequirementState(Status.FAIL, "Compiler is not loaded.", fixable=True)

        if compiler.running_image_name != compiler.expected_image_name:
            return RequirementState(
                Status.FAIL, f"Compiler is not running the expected image {compiler.expected_image_name}.", fixable=True
            )

        if not compiler.is_running:
            return RequirementState(Status.FAIL, "Compiler is not running.", fixable=True)

        if not compiler.is_ready:
            return RequirementState(Status.FAIL, "Compiler has not been correctly prepared.", fixable=True)

        return RequirementState(Status.OK, "Compiler is running.")


class AWSConfig(Requirement):
    def check(self, ctx: Context, fix: bool) -> RequirementState:
        aws_config = Path("~/.aws/config").expanduser()
        if not aws_config.exists():
            return RequirementState(Status.FAIL, "AWS config not found.")

        profile_header = f"[profile {AWS_ACCOUNT}]"
        if profile_header not in aws_config.read_text():
            return RequirementState(
                Status.FAIL,
                f"AWS config for account {AWS_ACCOUNT} not found. Check https://datadoghq.atlassian.net/wiki/spaces/ENG/pages/2498068557/AWS+SSO+Getting+Started to setup your AWS config.",
            )

        return RequirementState(Status.OK, "AWS config is set up.")
