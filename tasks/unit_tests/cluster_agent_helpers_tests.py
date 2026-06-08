import unittest
from unittest.mock import MagicMock, patch

from tasks.cluster_agent_helpers import build_common
from tasks.schema.template import CORE_SCHEMA_FILE


class TestClusterAgentHelpersBuildCommon(unittest.TestCase):
    @patch("tasks.cluster_agent_helpers.schema_compress")
    @patch("tasks.cluster_agent_helpers.refresh_assets_common")
    @patch("tasks.cluster_agent_helpers.generate_template")
    @patch("tasks.cluster_agent_helpers.go_build")
    @patch("tasks.cluster_agent_helpers.get_build_flags")
    def test_main_cluster_agent_renders_dca_template(self, gbf, _go, gen, _refresh, _compress):
        gbf.return_value = ("ld", "gc", {})
        ctx = MagicMock()

        build_common(
            ctx,
            bin_path="./bin/datadog-cluster-agent",
            build_tags=["kubeapiserver"],
            bin_suffix="",
            rebuild=False,
            build_include=None,
            build_exclude=None,
            race=False,
            development=False,
            skip_assets=True,
        )

        gen.assert_called_once_with(
            CORE_SCHEMA_FILE,
            "./Dockerfiles/cluster-agent/datadog-cluster.yaml",
            "dca",
            "linux",
        )

    @patch("tasks.cluster_agent_helpers.schema_compress")
    @patch("tasks.cluster_agent_helpers.refresh_assets_common")
    @patch("tasks.cluster_agent_helpers.generate_template")
    @patch("tasks.cluster_agent_helpers.go_build")
    @patch("tasks.cluster_agent_helpers.get_build_flags")
    def test_cloudfoundry_renders_dcacf_template(self, gbf, _go, gen, _refresh, _compress):
        gbf.return_value = ("ld", "gc", {})
        ctx = MagicMock()

        build_common(
            ctx,
            bin_path="./bin/datadog-cluster-agent-cloudfoundry",
            build_tags=["clusterchecks"],
            bin_suffix="-cloudfoundry",
            rebuild=False,
            build_include=None,
            build_exclude=None,
            race=False,
            development=False,
            skip_assets=True,
        )

        gen.assert_called_once_with(
            CORE_SCHEMA_FILE,
            "./cloudfoundry.yaml",
            "dcacf",
            "linux",
        )


if __name__ == "__main__":
    unittest.main()
