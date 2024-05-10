from __future__ import annotations

from pathlib import Path

from tasks.libs.types.arch import Arch, KMTArchName

KMT_SUPPORTED_ARCHS: list[KMTArchName] = ["x86_64", "arm64"]
VMCONFIG = "vmconfig.json"


class KMTPaths:
    def __init__(self, stack: str | None, arch: Arch):
        self.stack = stack
        self.arch = arch

    @property
    def repo_root(self):
        # this file is tasks/kernel_matrix_testing/vars.py, so two parents is the agent folder
        return Path(__file__).parent.parent.parent

    @property
    def root(self):
        return self.repo_root / "kmt-deps"

    @property
    def arch_dir(self):
        return self.stack_dir / self.arch.kmt_arch

    @property
    def stack_dir(self):
        if self.stack is None:
            raise RuntimeError("no stack name provided, cannot use stack-specific paths")

        return self.root / self.stack

    @property
    def dependencies(self):
        return self.arch_dir / "opt/testing-tools"

    @property
    def sysprobe_tests(self):
        return self.arch_dir / "opt/system-probe-tests"

    @property
    def secagent_tests(self):
        return self.arch_dir / "opt/security-agent-tests"

    @property
    def tools(self):
        return self.root / self.arch.kmt_arch / "tools"
