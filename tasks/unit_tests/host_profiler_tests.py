import os
import unittest
from unittest.mock import Mock, patch

import semver

from tasks import host_profiler


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
