import os
import unittest
from typing import TYPE_CHECKING, cast
from unittest.mock import MagicMock, patch

from tasks.kernel_matrix_testing import compiler, platforms, vmconfig
from tasks.kernel_matrix_testing.vars import KMT_SUPPORTED_ARCHS
from tasks.libs.types.arch import Arch

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import Component
    from tasks.libs.types.arch import KMTArchName


class TestVmconfig(unittest.TestCase):
    def test_all_list_possible__items_map_to_existing_platforms(self):
        possible = vmconfig.list_possible()
        plats = platforms.get_platforms()

        for name in possible:
            # Only test distros, not custom kernels
            if "distro" not in name:
                continue

            vmdef = vmconfig.normalize_vm_def(possible, name)
            _, version, arch = vmdef

            if arch == "local":
                arch = Arch.local().kmt_arch

            self.assertIn(arch, plats, f"{name} selects architecture {arch} which does not exist in the platform list")
            self.assertIn(
                version,
                plats[cast("KMTArchName", arch)],
                f"{name} maps to {version} which is not a valid version for architecture {arch}",
            )

    def test_normalize_vm_def__returns_expected_values(self):
        possible = vmconfig.list_possible()

        cases = [
            ("ubuntu22-arm64-distro", ("distro", "ubuntu_22.04", "arm64")),
            ("ubuntu22-x86_64-distro", ("distro", "ubuntu_22.04", "x86_64")),
            ("focal-arm64-distro", ("distro", "ubuntu_20.04", "arm64")),
            ("focal-x86_64-distro", ("distro", "ubuntu_20.04", "x86_64")),
            ("ubuntu_22-arm64-distro", ("distro", "ubuntu_22.04", "arm64")),
            ("ubuntu-22-x86_64-distro", ("distro", "ubuntu_22.04", "x86_64")),
        ]

        for input, expected in cases:
            self.assertEqual(vmconfig.normalize_vm_def(possible, input), expected)


class TestFilterByCIComponent(unittest.TestCase):
    """The generated vmconfig decides which microVMs get booted on the metal instances.

    Any (test set, architecture) pair that has no CI job would boot VMs nothing ever
    connects to, while still eating vCPU and memory on a heavily oversubscribed box.
    """

    components: "list[Component]" = ["security-agent", "system-probe"]

    def test_no_microvms_without_a_matching_ci_job(self):
        plats = platforms.get_platforms()

        for component in self.components:
            expected: dict[tuple[str, str], set[str]] = {}
            for job in platforms.get_ci_test_jobs(component):
                for test_set in job.test_set:
                    expected.setdefault((test_set, job.arch), set()).update(job.kernels)

            by_set = platforms.filter_by_ci_component(plats, component)
            for test_set, plat in by_set.items():
                for arch in KMT_SUPPORTED_ARCHS:
                    self.assertEqual(
                        set(plat[arch].keys()),
                        expected.get((test_set, arch), set()),
                        f"{component}: vmset {test_set} on {arch} does not match the kernels "
                        "of the CI jobs running that test set",
                    )

    def test_every_ci_job_gets_its_microvms(self):
        plats = platforms.get_platforms()

        for component in self.components:
            by_set = platforms.filter_by_ci_component(plats, component)
            for job in platforms.get_ci_test_jobs(component):
                for test_set in job.test_set:
                    self.assertIn(test_set, by_set, f"{component}: job {job.name} has no vmset")
                    self.assertTrue(
                        job.kernels.issubset(by_set[test_set][job.arch].keys()),
                        f"{component}: job {job.name} is missing microVMs for "
                        f"{job.kernels - set(by_set[test_set][job.arch].keys())}",
                    )


def _result(ok: bool, stdout: str = "", stderr: str = "") -> MagicMock:
    return MagicMock(ok=ok, stdout=stdout, stderr=stderr)


class TestGetBuildbarnToken(unittest.TestCase):
    def setUp(self):
        env = {k: v for k, v in os.environ.items() if k not in ("BUILDBARN_ID_TOKEN", "DD_BAZEL_REMOTE_CACHE")}
        self.enterContext(patch.dict(os.environ, env, clear=True))
        self.which = self.enterContext(patch.object(compiler.shutil, "which", return_value="/usr/bin/vault"))
        self.isatty = self.enterContext(patch.object(compiler.sys.stdin, "isatty", return_value=False))
        self.warn = self.enterContext(patch.object(compiler, "warn"))
        self.wants_cache = self.enterContext(patch.object(compiler, "_host_wants_remote_cache", return_value=True))
        self.ctx = MagicMock()

    def test_env_token_is_used_without_vault(self):
        os.environ["BUILDBARN_ID_TOKEN"] = "from-env"
        self.assertEqual(compiler.get_buildbarn_token(self.ctx), "from-env")
        self.ctx.run.assert_not_called()
        self.wants_cache.assert_not_called()

    def test_remote_cache_opt_out_skips_minting(self):
        os.environ["DD_BAZEL_REMOTE_CACHE"] = "off"
        os.environ["BUILDBARN_ID_TOKEN"] = "from-env"
        self.assertIsNone(compiler.get_buildbarn_token(self.ctx))
        self.ctx.run.assert_not_called()

    def test_ineligible_host_skips_vault(self):
        self.wants_cache.return_value = False
        self.assertIsNone(compiler.get_buildbarn_token(self.ctx))
        self.ctx.run.assert_not_called()
        self.warn.assert_not_called()

    def test_vault_read(self):
        self.ctx.run.return_value = _result(True, "minted\n")
        self.assertEqual(compiler.get_buildbarn_token(self.ctx), "minted")
        self.assertEqual(self.ctx.run.call_count, 1)

    def test_missing_vault_cli(self):
        self.which.return_value = None
        self.assertIsNone(compiler.get_buildbarn_token(self.ctx))
        self.ctx.run.assert_not_called()
        self.wants_cache.assert_not_called()

    def test_read_failure_without_tty_does_not_login(self):
        self.ctx.run.return_value = _result(False, stderr="Code: 403. Errors:\n\t* invalid token\n")
        self.assertIsNone(compiler.get_buildbarn_token(self.ctx))
        self.assertEqual(self.ctx.run.call_count, 1)
        self.assertIn("(* invalid token)", self.warn.call_args.args[0])

    def test_read_failure_with_tty_logs_in_on_host(self):
        self.isatty.return_value = True
        self.ctx.run.side_effect = [_result(False), _result(True), _result(True, "minted")]
        self.assertEqual(compiler.get_buildbarn_token(self.ctx), "minted")
        self.assertIn("vault login", self.ctx.run.call_args_list[1].args[0])


class TestHostWantsRemoteCache(unittest.TestCase):
    def test_true_when_selector_emits_config_cache(self):
        ctx = MagicMock()
        ctx.run.return_value = _result(True, "--config=cache\n")
        self.assertTrue(compiler._host_wants_remote_cache(ctx))
        cmd, kwargs = ctx.run.call_args.args[0], ctx.run.call_args.kwargs
        self.assertIn("remote-cache-select.sh", cmd)
        self.assertIn("_remote_cache_config", cmd)
        self.assertEqual(kwargs["env"]["BUILDBARN_ID_TOKEN"], "probe")

    def test_false_when_selector_emits_nothing(self):
        ctx = MagicMock()
        ctx.run.return_value = _result(True, "")
        self.assertFalse(compiler._host_wants_remote_cache(ctx))


class TestCompilerExecBuildbarnToken(unittest.TestCase):
    def setUp(self):
        env = {k: v for k, v in os.environ.items() if k != "DD_BAZEL_REMOTE_CACHE"}
        self.enterContext(patch.dict(os.environ, env, clear=True))
        self.ctx = MagicMock()
        self.cc = compiler.CompilerImage(self.ctx, Arch.local())
        self.enterContext(patch.object(compiler.CompilerImage, "ensure_running"))
        self.enterContext(patch.object(compiler.CompilerImage, "ensure_in_git_repo"))
        self.get_token = self.enterContext(patch.object(compiler, "get_buildbarn_token", return_value="secret"))

    def test_token_is_forwarded_by_name_only(self):
        self.cc.exec("bazel build //...", user="dev", buildbarn_token=True)
        cmd = self.ctx.run.call_args.args[0]
        self.assertIn("-e BUILDBARN_ID_TOKEN ", cmd)
        self.assertNotIn("secret", cmd)
        self.assertEqual(self.ctx.run.call_args.kwargs["env"], {"BUILDBARN_ID_TOKEN": "secret"})

    def test_no_token_unless_requested(self):
        self.cc.exec("true", user="dev")
        self.get_token.assert_not_called()
        self.assertNotIn("BUILDBARN_ID_TOKEN", self.ctx.run.call_args.args[0])
        self.assertEqual(self.ctx.run.call_args.kwargs["env"], {})

    def test_unavailable_token_is_not_forwarded(self):
        self.get_token.return_value = None
        self.cc.exec("bazel build //...", user="dev", buildbarn_token=True)
        self.assertNotIn("BUILDBARN_ID_TOKEN", self.ctx.run.call_args.args[0])
        self.assertEqual(self.ctx.run.call_args.kwargs["env"], {})

    def test_cache_policy_is_forwarded_to_builds(self):
        os.environ["DD_BAZEL_REMOTE_CACHE"] = "off"
        self.get_token.return_value = None
        self.cc.exec("bazel build //...", user="dev", buildbarn_token=True)
        self.assertIn("-e DD_BAZEL_REMOTE_CACHE ", self.ctx.run.call_args.args[0])
        self.cc.exec("true", user="dev")
        self.assertNotIn("DD_BAZEL_REMOTE_CACHE", self.ctx.run.call_args.args[0])


class TestCompilerUser(unittest.TestCase):
    def setUp(self):
        self.ctx = MagicMock()
        self.cc = compiler.CompilerImage(self.ctx, Arch.local())

    def _security_options(self, options: str):
        self.ctx.run.side_effect = lambda cmd, **_: (
            _result(True, options) if cmd.startswith("docker info") else _result(True, "502\n")
        )

    def test_rootless_engine_builds_as_root(self):
        self._security_options('["name=seccomp,profile=default","name=rootless"]')
        self.assertTrue(self.cc.is_rootless)
        self.assertEqual((self.cc.compiler_uid, self.cc.compiler_gid), ("0", "0"))

    def test_rootful_engine_builds_as_host_user(self):
        self._security_options('["name=seccomp,profile=default"]')
        self.assertFalse(self.cc.is_rootless)
        self.assertEqual((self.cc.compiler_uid, self.cc.compiler_gid), ("502", "502"))

    def test_user_and_home_come_from_container_passwd(self):
        self.enterContext(patch.object(compiler.CompilerImage, "compiler_uid", "0"))
        exec_ = self.enterContext(
            patch.object(
                compiler.CompilerImage, "exec", return_value=_result(True, "root:x:0:0:root:/root:/bin/bash\n")
            )
        )
        self.assertEqual((self.cc.compiler_user, self.cc.compiler_home), ("root", "/root"))
        exec_.assert_called_once_with("getent passwd 0", user="root")
