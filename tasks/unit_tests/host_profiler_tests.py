import os
import unittest
from unittest.mock import Mock, patch

import semver

from tasks import host_profiler


class TestBuild(unittest.TestCase):
    def test_default_native_optimization_when_cgo_cflags_unset(self):
        # If CGO_CFLAGS is left empty by the user, build must restore Go's
        # default -O2 -g that setting the env var would otherwise replace.
        with (
            patch.dict(os.environ, {}, clear=True),
            patch.object(host_profiler.sys, "platform", "linux"),
            patch.object(host_profiler.os.path, "exists", return_value=False),
            patch.object(host_profiler.os, "mkdir"),
            patch.object(host_profiler.shutil, "copy"),
            patch.object(host_profiler, "_get_profiler_agent_version", return_value=None),
            patch.object(
                host_profiler,
                "get_build_flags",
                return_value=("", "", {"CGO_CFLAGS": " -Werror -Wno-deprecated-declarations"}),
            ),
            patch.object(host_profiler, "go_build") as go_build,
        ):
            host_profiler.build(host_profiler.Context())

        self.assertEqual(
            go_build.call_args.kwargs["env"]["CGO_CFLAGS"], "-O2 -g -Werror -Wno-deprecated-declarations"
        )


class TestGetProfilerAgentVersion(unittest.TestCase):
    def test_preview_version_returned_by_function_is_semver(self):
        with (
            patch.dict(
                os.environ,
                {
                    "BUCKET_BRANCH": "dev",
                    "CI_COMMIT_REF_SLUG": "hp-preview-1-0",
                },
                clear=True,
            ),
            patch.object(host_profiler, "get_current_milestone", return_value="7.81.0"),
            patch.object(
                host_profiler,
                "query_version",
                return_value=("7.81.0", "", 356, "09ad4bc", None),
            ),
        ):
            version = host_profiler._get_profiler_agent_version(Mock())

        # generated version should always be semver compliant (no '_' for example)
        self.assertTrue(semver.VersionInfo.isvalid(version))
