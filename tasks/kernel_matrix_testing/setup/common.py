import grp
import os
import pwd
import re
import shutil
from pathlib import Path

from invoke.context import Context

from tasks.kernel_matrix_testing.compiler import get_compiler
from tasks.libs.common.status import Status
from tasks.libs.common.utils import get_repo_root, is_installed

from .requirement import Requirement, RequirementState


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
    def get_candidate_paths() -> list[Path]:
        # Allow callers to force a specific path
        env_path = os.environ.get("KMT_TEST_INFRA_DEFINITIONS_PATH")
        if env_path is not None:
            return [Path(env_path)]

        # Default to common paths if a specific setting has not been forced
        return [
            get_repo_root().parent / "test-infra-definitions",
            Path("~/go/src/github.com/DataDog/test-infra-definitions").expanduser(),
        ]

    @staticmethod
    def get_repo_path() -> Path | None:
        for path in TestInfraDefinitionsRepo.get_candidate_paths():
            if path.is_dir():
                return path

        return None

    def check(self, ctx: Context, fix: bool) -> RequirementState:
        repo_path = self.get_repo_path()
        if repo_path is not None:
            return RequirementState(Status.OK, "test-infra-definitions repository found.")

        candidate_paths = TestInfraDefinitionsRepo.get_candidate_paths()
        if not fix:
            return RequirementState(
                Status.FAIL,
                f"test-infra-definitions repository not found in any of the expected locations {candidate_paths}.",
                fixable=True,
            )

        clone_opts = "--depth 1 --single-branch --branch=main" if os.environ.get("CI") else ""
        repo_access = "git+https://github.com/" if os.environ.get("CI") else "git@github.com:"

        try:
            ctx.run(f"git clone {repo_access}DataDog/test-infra-definitions.git {candidate_paths[0]} {clone_opts}")
        except Exception as e:
            return RequirementState(Status.FAIL, f"test-infra-definitions could not be cloned: {e}", fixable=True)

        return RequirementState(Status.OK, "test-infra-definitions repository cloned.")


class PulumiPlugin(Requirement):
    dependencies: list[type[Requirement]] = [Pulumi, TestInfraDefinitionsRepo]

    def check(self, ctx: Context, fix: bool) -> RequirementState:
        test_infra_repo = TestInfraDefinitionsRepo.get_repo_path()
        if test_infra_repo is None:
            return RequirementState(Status.FAIL, "test-infra-definitions repository not found.")

        with ctx.cd(test_infra_repo):
            res = ctx.run("pulumi --non-interactive plugin ls", warn=True)
            # If there are more than 3 lines, then there are plugins installed. The other lines are headers/footers.
            if res is not None and res.ok and len(res.stdout.splitlines()) > 3:
                return RequirementState(Status.OK, "pulumi plugins installed.")

            if not fix:
                return RequirementState(Status.FAIL, "pulumi plugins not installed.", fixable=True)

            try:
                ctx.run("go mod download")
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
                ctx.run("dda self dep update")
            except Exception as e:
                return RequirementState(Status.FAIL, f"Failed to update dda: {e}")

        return RequirementState(Status.OK, f"dda is installed (version {dda_version.stdout.strip()} >= {min_version}).")


class PythonDependenciesRequirement(Requirement):
    dependencies: list[type[Requirement]] = [DDA]

    def check(self, ctx: Context, fix: bool) -> RequirementState:
        installed_dependencies = ctx.run("dda self dep show --legacy", warn=True)
        if installed_dependencies is None or not installed_dependencies.ok:
            return RequirementState(
                Status.FAIL, "dda is too old or not installed correctly, cannot get installed dependencies."
            )

        if 'legacy-kernel-matrix-testing' in installed_dependencies.stdout:
            return RequirementState(Status.OK, "Python dependencies are already installed.")

        if not fix:
            return RequirementState(Status.FAIL, "Python dependencies not installed.", fixable=True)

        try:
            ctx.run("dda inv --feat legacy-kernel-matrix-testing -- --help", hide=True)
        except Exception as e:
            return RequirementState(Status.FAIL, f"Failed to install Python dependencies: {e}")

        return RequirementState(Status.OK, "Python dependencies installed.")


class KMTDirectoriesRequirement(Requirement):
    def check(self, ctx: Context, fix: bool) -> list[RequirementState]:
        import getpass

        from tasks.kernel_matrix_testing.kmt_os import get_kmt_os
        from tasks.kernel_matrix_testing.tool import is_root

        states = []
        kmt_os = get_kmt_os()
        sudo = "sudo" if not is_root() else ""
        dirs = [
            kmt_os.shared_dir,
            kmt_os.kmt_dir,
            kmt_os.packages_dir,
            kmt_os.stacks_dir,
            kmt_os.libvirt_dir,
            kmt_os.rootfs_dir,
            kmt_os.shared_dir,
        ]

        user = getpass.getuser()
        user_id = pwd.getpwnam(user).pw_uid
        group_id = grp.getgrnam(kmt_os.libvirt_group).gr_gid

        for d in dirs:
            exists = d.exists()

            if not exists:
                if not fix:
                    states.append(RequirementState(Status.FAIL, f"Directory {d} does not exist.", fixable=True))
                else:
                    ctx.run(f"{sudo} install -d -m 0755 -g {kmt_os.libvirt_group} -o {user} {d}")
                    states.append(RequirementState(Status.OK, f"Created missing KMT directory: {d}"))

            perms = d.stat().st_mode
            if perms & 0o777 != 0o755:  # Check only the permission bits
                if not fix:
                    states.append(
                        RequirementState(
                            Status.FAIL, f"Directory {d} has incorrect permissions {oct(perms)}.", fixable=True
                        )
                    )
                else:
                    ctx.run(f"{sudo} chmod 0755 {d}")
                    states.append(RequirementState(Status.OK, f"Fixed permissions for KMT directory: {d}"))

            owner_id = d.stat().st_uid
            group_id = d.stat().st_gid
            if owner_id != user_id or group_id != group_id:
                if not fix:
                    states.append(
                        RequirementState(
                            Status.FAIL,
                            f"Directory {d} has incorrect owner or group (owner ID={owner_id}, group ID={group_id}).",
                            fixable=True,
                        )
                    )
                else:
                    ctx.run(f"{sudo} chown -R {user}:{kmt_os.libvirt_group} {d}")
                    states.append(RequirementState(Status.OK, f"Fixed owner and group for KMT directory: {d}"))

        return states


class KMTSSHKey(Requirement):
    def check(self, ctx: Context, fix: bool) -> RequirementState:
        from tasks.kernel_matrix_testing.kmt_os import get_kmt_os

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


class Compiler(Requirement):
    def check(self, ctx: Context, fix: bool) -> RequirementState:
        compiler = get_compiler(ctx)
        if compiler.is_running:
            return RequirementState(Status.OK, "Compiler is running.")

        if not fix:
            return RequirementState(Status.FAIL, "Compiler is not running.", fixable=True)

        compiler.start()
        return RequirementState(Status.OK, "Compiler started.")
