import os
import unittest
from types import SimpleNamespace
from unittest.mock import patch

from invoke.context import Context
from invoke.runners import Runner

from tasks.libs.common import go_toolchain
from tasks.libs.common.go_toolchain import _GO_COMMAND, inject_go_toolchain, install_go_toolchain_hook


class TestGoCommandDetection(unittest.TestCase):
    def test_matches(self):
        for command in (
            "go build ./cmd/agent",
            "go.exe test ./...",
            "GOOS=windows GOARCH=amd64 go build .",
            "cd pkg/foo && go mod tidy",
            "go",
        ):
            self.assertRegex(command, _GO_COMMAND)

    def test_does_not_match(self):
        for command in (
            "gofmt -l .",
            "golangci-lint run",
            "cargo build",
            "bazelisk run //foo -- go version",
        ):
            self.assertNotRegex(command, _GO_COMMAND)


class TestInjectGoToolchain(unittest.TestCase):
    def setUp(self):
        go_toolchain._injected = False
        self.addCleanup(setattr, go_toolchain, "_injected", False)
        self.addCleanup(os.environ.update, {"PATH": os.environ["PATH"]})

    @patch("tasks.libs.common.go_toolchain.hermetic_go_bin_dir", return_value="/sdk/bin")
    def test_prepends_once(self, bin_dir):
        inject_go_toolchain(None)
        inject_go_toolchain(None)

        self.assertTrue(os.environ["PATH"].startswith("/sdk/bin" + os.pathsep))
        self.assertEqual(bin_dir.call_count, 1)

    @patch("tasks.libs.common.go_toolchain.hermetic_go_bin_dir", side_effect=RuntimeError("no bazel"))
    def test_falls_back_to_host_go(self, _):
        path = os.environ["PATH"]

        self.assertIsNone(inject_go_toolchain(None))
        self.assertEqual(os.environ["PATH"], path)


class TestRunnerHook(unittest.TestCase):
    def setUp(self):
        # Pretend the SDK has already been located, so no Bazel call is needed.
        go_toolchain._injected, go_toolchain._go_bin_dir = True, "/sdk/bin"
        go_toolchain._hook_installed = False
        self.addCleanup(setattr, go_toolchain, "_injected", False)
        self.addCleanup(setattr, go_toolchain, "_go_bin_dir", None)
        self.addCleanup(setattr, go_toolchain, "_hook_installed", False)

    def run_through_hook(self, command, **kwargs):
        runner = SimpleNamespace(context=Context())
        with patch.object(Runner, "run") as inner:
            install_go_toolchain_hook()
            Runner.run(runner, command, **kwargs)
        return inner.call_args.kwargs

    def test_prepends_to_caller_supplied_path(self):
        kwargs = self.run_through_hook("go build ./cmd/agent", env={"PATH": "/mingw"})

        self.assertEqual(kwargs["env"]["PATH"], "/sdk/bin" + os.pathsep + "/mingw")

    def test_leaves_other_commands_alone(self):
        kwargs = self.run_through_hook("gofmt -l .", env={"PATH": "/mingw"})

        self.assertEqual(kwargs["env"]["PATH"], "/mingw")
