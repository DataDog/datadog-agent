import os
import unittest
from unittest.mock import ANY, MagicMock, patch

from invoke import MockContext, Result
from invoke.exceptions import Exit

from tasks.libs.package.size import InfraError
from tasks.quality_gates import display_pr_comment, generate_new_quality_gate_config, parse_and_trigger_gates
from tasks.static_quality_gates.lib.gates_lib import (
    GateMetricHandler,
    StaticQualityGateDocker,
    StaticQualityGateFailed,
    StaticQualityGatePackage,
)


class MockMetricHandler:
    def __init__(self, metrics):
        self.metrics = metrics
        self.total_size_saved = 0


class TestQualityGatesConfigUpdate(unittest.TestCase):
    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
            'CI_COMMIT_REF_SLUG': 'pikachu',
            'CI_COMMIT_SHORT_SHA': '1234567890',
            'BUCKET_BRANCH': 'main',
            'OMNIBUS_PACKAGE_DIR': '/opt/datadog-agent',
        },
    )
    @patch(
        "tasks.static_quality_gates.lib.StaticQualityGatePackage._find_package_path",
        new=MagicMock(),
    )
    @patch(
        "tasks.static_quality_gates.lib.StaticQualityGatePackage._calculate_package_size",
        new=MagicMock(),
    )
    @patch(
        "tasks.static_quality_gates.lib.StaticQualityGateDocker._calculate_image_on_wire_size",
        new=MagicMock(),
    )
    @patch(
        "tasks.static_quality_gates.lib.StaticQualityGateDocker._calculate_image_on_disk_size",
        new=MagicMock(),
    )
    @patch("tasks.static_quality_gates.lib.gates_lib.GateMetricHandler.send_metrics_to_datadog", new=MagicMock())
    def test_parse_and_trigger_gates_infra_error(self):
        ctx = MockContext(
            run={
                "datadog-ci tag --level job --tags static_quality_gates:\"restart\"": Result("Done"),
                "datadog-ci tag --level job --tags static_quality_gates:\"failure\"": Result("Done"),
            }
        )
        mock_quality_gates_module = MagicMock()
        mock_quality_gates_module.static_quality_gate_agent_suse_amd64.execute_gate.side_effect = InfraError(
            "Test infra error message"
        )
        with self.assertRaises(Exit) as cm:
            parse_and_trigger_gates(ctx, "tasks/unit_tests/testdata/quality_gate_config_test.yml")
            assert "Test infra error message" in str(cm.exception)

    def test_one_gate_update(self):
        with open("tasks/unit_tests/testdata/quality_gate_config_test.yml") as f:
            new_config, saved_amount = generate_new_quality_gate_config(
                f,
                MockMetricHandler(
                    {
                        "static_quality_gate_agent_suse_amd64": {
                            "current_on_wire_size": 50000000,
                            "max_on_wire_size": 100000000,
                            "current_on_disk_size": 50000000,
                            "max_on_disk_size": 100000000,
                        },
                        "static_quality_gate_agent_deb_amd64": {
                            "current_on_wire_size": 4000000,
                            "max_on_wire_size": 5000000,
                            "current_on_disk_size": 4000000,
                            "max_on_disk_size": 5000000,
                        },
                        "static_quality_gate_docker_agent_amd64": {
                            "current_on_wire_size": 50000000,
                            "max_on_wire_size": 100000000,
                            "current_on_disk_size": 50000000,
                            "max_on_disk_size": 100000000,
                        },
                    }
                ),
            )
        assert new_config["static_quality_gate_agent_suse_amd64"]["max_on_wire_size"] == "48.64 MiB", print(
            f"Expected 48.64 MiB got {new_config['static_quality_gate_agent_suse_amd64']['max_on_wire_size']}"
        )
        assert new_config["static_quality_gate_agent_suse_amd64"]["max_on_disk_size"] == "48.64 MiB", print(
            f"Expected 48.64 MiB got {new_config['static_quality_gate_agent_suse_amd64']['max_on_disk_size']}"
        )
        assert new_config["static_quality_gate_agent_deb_amd64"]["max_on_wire_size"] == "4.77 MiB", print(
            f"Expected 4.77 MiB got {new_config['static_quality_gate_agent_deb_amd64']['max_on_wire_size']}"
        )
        assert new_config["static_quality_gate_agent_deb_amd64"]["max_on_disk_size"] == "4.77 MiB", print(
            f"Expected 4.77 MiB got {new_config['static_quality_gate_agent_deb_amd64']['max_on_disk_size']}"
        )

    def test_exception_gate_bump(self):
        with open("tasks/unit_tests/testdata/quality_gate_config_test.yml") as f:
            new_config, saved_amount = generate_new_quality_gate_config(
                f,
                MockMetricHandler(
                    {
                        "static_quality_gate_agent_suse_amd64": {
                            "relative_on_wire_size": 424242,
                            "current_on_wire_size": 50000000,
                            "max_on_wire_size": 100000000,
                            "relative_on_disk_size": 242424,
                            "current_on_disk_size": 50000000,
                            "max_on_disk_size": 100000000,
                        },
                        "static_quality_gate_agent_deb_amd64": {
                            "relative_on_wire_size": 424242,
                            "current_on_wire_size": 4000000,
                            "max_on_wire_size": 5000000,
                            "relative_on_disk_size": 242424,
                            "current_on_disk_size": 4000000,
                            "max_on_disk_size": 5000000,
                        },
                        "static_quality_gate_docker_agent_amd64": {
                            "relative_on_wire_size": 424242,
                            "current_on_wire_size": 50000000,
                            "max_on_wire_size": 100000000,
                            "current_on_disk_size": 50000000,
                            "relative_on_disk_size": 242424,
                            "max_on_disk_size": 100000000,
                        },
                    }
                ),
                True,
            )
        assert new_config["static_quality_gate_agent_suse_amd64"]["max_on_wire_size"] == "95.77 MiB", print(
            f"Expected 48.64 MiB got {new_config['static_quality_gate_agent_suse_amd64']['max_on_wire_size']}"
        )
        assert new_config["static_quality_gate_agent_suse_amd64"]["max_on_disk_size"] == "95.6 MiB", print(
            f"Expected 48.64 MiB got {new_config['static_quality_gate_agent_suse_amd64']['max_on_disk_size']}"
        )
        assert new_config["static_quality_gate_agent_deb_amd64"]["max_on_wire_size"] == "5.17 MiB", print(
            f"Expected 4.77 MiB got {new_config['static_quality_gate_agent_deb_amd64']['max_on_wire_size']}"
        )
        assert new_config["static_quality_gate_agent_deb_amd64"]["max_on_disk_size"] == "5.0 MiB", print(
            f"Expected 4.77 MiB got {new_config['static_quality_gate_agent_deb_amd64']['max_on_disk_size']}"
        )


class TestQualityGatesPrMessage(unittest.TestCase):
    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch(
        "tasks.static_quality_gates.lib.gates_lib.GateMetricHandler.get_formatted_metric",
        new=MagicMock(return_value="10MiB"),
    )
    @patch(
        "tasks.quality_gates.get_debug_job_url",
        new=MagicMock(return_value="https://gitlab.ddbuild.io/DataDog/datadog-agent/-/jobs/00000000"),
    )
    @patch("tasks.quality_gates.pr_commenter")
    def test_no_error(self, pr_commenter_mock):
        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        display_pr_comment(
            c,
            True,
            [
                {'name': 'gateA', 'error_type': None, 'message': None},
                {'name': 'gateB', 'error_type': None, 'message': None},
            ],
            gate_metric_handler,
            "value",
        )
        pr_commenter_mock.assert_called_once()
        pr_commenter_mock.assert_called_with(
            ANY,
            title='Static quality checks',
            body='✅ Please find below the results from static quality gates\nComparison made with [ancestor](https://github.com/DataDog/datadog-agent/commit/value) value\n\n\n<details>\n<summary>Successful checks</summary>\n\n### Info\n\n||Quality gate|Delta|On disk size (MiB)|Delta|On wire size (MiB)|\n|--|--|--|--|--|--|\n|✅|gateA|10MiB|DataNotFound|10MiB|DataNotFound|\n|✅|gateB|10MiB|DataNotFound|10MiB|DataNotFound|\n\n</details>\n',
        )

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch(
        "tasks.static_quality_gates.lib.gates_lib.GateMetricHandler.get_formatted_metric",
        new=MagicMock(return_value="10MiB"),
    )
    @patch(
        "tasks.quality_gates.get_debug_job_url",
        new=MagicMock(return_value="https://gitlab.ddbuild.io/DataDog/datadog-agent/-/jobs/00000000"),
    )
    @patch("tasks.quality_gates.pr_commenter")
    def test_no_info(self, pr_commenter_mock):
        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        display_pr_comment(
            c,
            False,
            [
                {'name': 'gateA', 'error_type': 'AssertionError', 'message': 'some_msg_A'},
                {'name': 'gateB', 'error_type': 'AssertionError', 'message': 'some_msg_B'},
            ],
            gate_metric_handler,
            "value",
        )
        pr_commenter_mock.assert_called_once()
        pr_commenter_mock.assert_called_with(
            ANY,
            title='Static quality checks',
            body='❌ Please find below the results from static quality gates\nComparison made with [ancestor](https://github.com/DataDog/datadog-agent/commit/value) value\n### Error\n\n||Quality gate|Delta|On disk size (MiB)|Delta|On wire size (MiB)|\n|--|--|--|--|--|--|\n|❌|gateA|10MiB|DataNotFound|10MiB|DataNotFound|\n|❌|gateB|10MiB|DataNotFound|10MiB|DataNotFound|\n<details>\n<summary>Gate failure full details</summary>\n\n|Quality gate|Error type|Error message|\n|----|---|--------|\n|gateA|AssertionError|some_msg_A|\n|gateB|AssertionError|some_msg_B|\n\n</details>\n\nStatic quality gates prevent the PR to merge! \nTo understand the size increase caused by this PR, feel free to use the [debug_static_quality_gates](https://gitlab.ddbuild.io/DataDog/datadog-agent/-/jobs/00000000) manual gitlab job to compare what this PR introduced for a specific gate.\nUsage:\n- Run the manual job with the following Key / Value pair as CI/CD variable on the gitlab UI. Example for amd64 deb packages\nKey: `GATE_NAME`, Value: `static_quality_gate_agent_deb_amd64`\n\nYou can check the static quality gates [confluence page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4805854687/Static+Quality+Gates) for guidance. We also have a [toolbox page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4887448722/Static+Quality+Gates+Toolbox) available to list tools useful to debug the size increase.\n\n\n',
        )

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch(
        "tasks.static_quality_gates.lib.gates_lib.GateMetricHandler.get_formatted_metric",
        new=MagicMock(return_value="10MiB"),
    )
    @patch(
        "tasks.quality_gates.get_debug_job_url",
        new=MagicMock(return_value="https://gitlab.ddbuild.io/DataDog/datadog-agent/-/jobs/00000000"),
    )
    @patch("tasks.quality_gates.pr_commenter")
    def test_one_of_each(self, pr_commenter_mock):
        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        display_pr_comment(
            c,
            False,
            [
                {'name': 'gateA', 'error_type': 'AssertionError', 'message': 'some_msg_A'},
                {'name': 'gateB', 'error_type': None, 'message': None},
            ],
            gate_metric_handler,
            "value",
        )
        pr_commenter_mock.assert_called_once()
        pr_commenter_mock.assert_called_with(
            ANY,
            title='Static quality checks',
            body='❌ Please find below the results from static quality gates\nComparison made with [ancestor](https://github.com/DataDog/datadog-agent/commit/value) value\n### Error\n\n||Quality gate|Delta|On disk size (MiB)|Delta|On wire size (MiB)|\n|--|--|--|--|--|--|\n|❌|gateA|10MiB|DataNotFound|10MiB|DataNotFound|\n<details>\n<summary>Gate failure full details</summary>\n\n|Quality gate|Error type|Error message|\n|----|---|--------|\n|gateA|AssertionError|some_msg_A|\n\n</details>\n\nStatic quality gates prevent the PR to merge! \nTo understand the size increase caused by this PR, feel free to use the [debug_static_quality_gates](https://gitlab.ddbuild.io/DataDog/datadog-agent/-/jobs/00000000) manual gitlab job to compare what this PR introduced for a specific gate.\nUsage:\n- Run the manual job with the following Key / Value pair as CI/CD variable on the gitlab UI. Example for amd64 deb packages\nKey: `GATE_NAME`, Value: `static_quality_gate_agent_deb_amd64`\n\nYou can check the static quality gates [confluence page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4805854687/Static+Quality+Gates) for guidance. We also have a [toolbox page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4887448722/Static+Quality+Gates+Toolbox) available to list tools useful to debug the size increase.\n\n\n<details>\n<summary>Successful checks</summary>\n\n### Info\n\n||Quality gate|Delta|On disk size (MiB)|Delta|On wire size (MiB)|\n|--|--|--|--|--|--|\n|✅|gateB|10MiB|DataNotFound|10MiB|DataNotFound|\n\n</details>\n',
        )

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch(
        "tasks.static_quality_gates.lib.gates_lib.GateMetricHandler.get_formatted_metric",
        new=MagicMock(return_value="10MiB", side_effect=KeyError),
    )
    @patch(
        "tasks.quality_gates.get_debug_job_url",
        new=MagicMock(return_value="https://gitlab.ddbuild.io/DataDog/datadog-agent/-/jobs/00000000"),
    )
    @patch("tasks.quality_gates.pr_commenter")
    def test_missing_data(self, pr_commenter_mock):
        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        display_pr_comment(
            c,
            False,
            [
                {'name': 'gateA', 'error_type': 'AssertionError', 'message': 'some_msg_A'},
            ],
            gate_metric_handler,
            "value",
        )
        pr_commenter_mock.assert_called_once()
        pr_commenter_mock.assert_called_with(
            ANY,
            title='Static quality checks',
            body='❌ Please find below the results from static quality gates\nComparison made with [ancestor](https://github.com/DataDog/datadog-agent/commit/value) value\n### Error\n\n||Quality gate|Delta|On disk size (MiB)|Delta|On wire size (MiB)|\n|--|--|--|--|--|--|\n|❌|gateA|DataNotFound|DataNotFound|DataNotFound|DataNotFound|\n<details>\n<summary>Gate failure full details</summary>\n\n|Quality gate|Error type|Error message|\n|----|---|--------|\n|gateA|AssertionError|some_msg_A|\n\n</details>\n\nStatic quality gates prevent the PR to merge! \nTo understand the size increase caused by this PR, feel free to use the [debug_static_quality_gates](https://gitlab.ddbuild.io/DataDog/datadog-agent/-/jobs/00000000) manual gitlab job to compare what this PR introduced for a specific gate.\nUsage:\n- Run the manual job with the following Key / Value pair as CI/CD variable on the gitlab UI. Example for amd64 deb packages\nKey: `GATE_NAME`, Value: `static_quality_gate_agent_deb_amd64`\n\nYou can check the static quality gates [confluence page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4805854687/Static+Quality+Gates) for guidance. We also have a [toolbox page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4887448722/Static+Quality+Gates+Toolbox) available to list tools useful to debug the size increase.\n\n\n',
        )


class TestOnDiskImageSizeCalculation(unittest.TestCase):
    def tearDown(self):
        try:
            os.remove('./tasks/unit_tests/testdata/fake_agent_image/with_tar_gz_archive/some_archive.tar.gz')
            os.remove('./tasks/unit_tests/testdata/fake_agent_image/with_tar_gz_archive/some_metadata.json')
        except OSError:
            pass
        try:
            os.remove('./tasks/unit_tests/testdata/fake_agent_image/without_tar_gz_archive/some_metadata.json')
        except OSError:
            pass


@patch.dict(
    'os.environ',
    {
        'CI_COMMIT_REF_NAME': 'pikachu',
        'CI_COMMIT_BRANCH': 'sequoia',
        'CI_COMMIT_REF_SLUG': 'pikachu',
        'CI_COMMIT_SHORT_SHA': '668844',
        'BUCKET_BRANCH': 'main',
        'OMNIBUS_PACKAGE_DIR': '/opt/datadog-agent',
        'CI_PIPELINE_ID': '71580015',
    },
)
@patch("glob.glob", new=MagicMock(return_value=["/opt/datadog-agent/datadog-agent_7*amd64.deb"]))
@patch(
    "tasks.static_quality_gates.lib.gates_lib.StaticQualityGatePackage._calculate_package_size",
    new=MagicMock(return_value=(100, 350)),
)
@patch(
    "tasks.static_quality_gates.lib.gates_lib.StaticQualityGateDocker._calculate_image_on_wire_size",
    new=MagicMock(return_value=(100, 350)),
)
@patch(
    "tasks.static_quality_gates.lib.gates_lib.StaticQualityGateDocker._calculate_image_on_disk_size",
    new=MagicMock(return_value=(100, 350)),
)
class TestGatesLib(unittest.TestCase):
    @patch("tasks.static_quality_gates.lib.gates_lib.GateMetricHandler", new=MagicMock())
    def test_get_quality_gates_list(self):
        from tasks.static_quality_gates.lib.gates_lib import get_quality_gates_list

        gates = get_quality_gates_list("tasks/static_quality_gates/lib/static_quality_gates_test.yaml", MagicMock())
        assert len(gates) == 22

    def test_set_arch(self):
        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_deb_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.arch, "amd64")
        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_deb_arm64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.arch, "arm64")
        gate = StaticQualityGatePackage(
            "static_quality_gate_iot_agent_rpm_armhf", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.arch, "armv7hl")
        with self.assertRaises(ValueError):
            gate = StaticQualityGatePackage(
                "static_quality_gate_agent_deb_unknown", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
            )

    def test_set_os(self):
        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_deb_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.os, "debian")
        self.assertEqual(gate.arch, "amd64")
        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_rpm_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.os, "centos")
        self.assertEqual(gate.arch, "x86_64")
        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_suse_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.os, "suse")

        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_heroku_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.os, "debian")
        self.assertEqual(gate.arch, "amd64")
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.os, "docker")
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_windows_2022_servercore_amd64",
            {"max_on_wire_size": 100, "max_on_disk_size": 100},
            MagicMock(),
        )
        self.assertEqual(gate.os, "docker")
        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_msi", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.os, "windows")
        self.assertEqual(gate.arch, "x86_64")
        with self.assertRaises(ValueError):
            gate = StaticQualityGatePackage(
                "static_quality_gate_agent_unknown_unknown",
                {"max_on_wire_size": 100, "max_on_disk_size": 100},
                MagicMock(),
            )

    def test_max_sizes(self):
        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_deb_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 350}, MagicMock()
        )
        self.assertEqual(gate.max_on_wire_size, 100)
        self.assertEqual(gate.max_on_disk_size, 350)

        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_amd64", {"max_on_wire_size": 125, "max_on_disk_size": 250}, MagicMock()
        )
        self.assertEqual(gate.max_on_wire_size, 125)
        self.assertEqual(gate.max_on_disk_size, 250)

    def test_find_package_path(self):
        test_cases = [
            ("static_quality_gate_agent_deb_amd64", "/opt/datadog-agent/datadog-agent_7*amd64.deb"),
            ("static_quality_gate_agent_deb_amd64_fips", "/opt/datadog-agent/datadog-fips-agent_7*amd64.deb"),
            ("static_quality_gate_iot_agent_rpm_arm64", "/opt/datadog-agent/datadog-iot-agent-7*aarch64.rpm"),
            ("static_quality_gate_dogstatsd_suse_amd64", "/opt/datadog-agent/datadog-dogstatsd-7*x86_64.rpm"),
            ("static_quality_gate_agent_heroku_amd64", "/opt/datadog-agent/datadog-heroku-agent_7*amd64.deb"),
        ]

        for gate_name, expected_pattern in test_cases:
            with self.subTest(gate_name=gate_name):
                mock_glob = MagicMock(return_value=["/opt/datadog-agent/some_package.ext"])

                with patch("glob.glob", mock_glob):
                    gate = StaticQualityGatePackage(
                        gate_name, {"max_on_wire_size": 100, "max_on_disk_size": 350}, MagicMock()
                    )
                    gate._find_package_path()

                actual_pattern = mock_glob.call_args[0][0]
                self.assertEqual(actual_pattern, expected_pattern)

    def test_find_package_path_msi(self):
        """Test MSI package path finding with dual-file discovery"""
        mock_glob = MagicMock()
        # First call returns ZIP file, second call returns MSI file
        mock_glob.side_effect = [
            ["/opt/datadog-agent/pipeline-123/datadog-agent-7.50.0-x86_64.zip"],
            ["/opt/datadog-agent/pipeline-123/datadog-agent-7.50.0-x86_64.msi"],
        ]

        with patch("glob.glob", mock_glob):
            gate = StaticQualityGatePackage(
                "static_quality_gate_agent_msi", {"max_on_wire_size": 100, "max_on_disk_size": 350}, MagicMock()
            )
            gate._find_package_path()

        # Verify both ZIP and MSI paths are set
        self.assertTrue(hasattr(gate, 'zip_path'))
        self.assertTrue(hasattr(gate, 'msi_path'))
        self.assertEqual(gate.zip_path, "/opt/datadog-agent/pipeline-123/datadog-agent-7.50.0-x86_64.zip")
        self.assertEqual(gate.msi_path, "/opt/datadog-agent/pipeline-123/datadog-agent-7.50.0-x86_64.msi")
        self.assertEqual(gate.artifact_path, gate.msi_path)  # MSI is the primary artifact

    def test_check_artifact_size(self):
        # Test case where quality gate passes - sizes are within limits
        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_deb_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 350}, MagicMock()
        )
        gate.artifact_on_wire_size = 100
        gate.artifact_on_disk_size = 350
        gate.check_artifact_size()

        # Test case where quality gate fails - sizes exceed maximum allowed
        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_deb_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 350}, MagicMock()
        )
        gate.artifact_on_wire_size = 150  # Exceeds max_on_wire_size of 100
        gate.artifact_on_disk_size = 400  # Exceeds max_on_disk_size of 350

        with self.assertRaises(StaticQualityGateFailed) as context:
            gate.check_artifact_size()

        # Verify the exception message contains information about both failures
        error_message = str(context.exception)
        # Values are converted to MB in error messages: 150 bytes ≈ 0.000143 MB, 100 bytes ≈ 9.54e-05 MB
        self.assertIn(
            "On wire size (compressed artifact size) 0.0001430511474609375 MB is higher than the maximum allowed 9.5367431640625e-05 MB by the gate !",
            error_message,
        )
        # 400 bytes ≈ 0.000381 MB, 350 bytes ≈ 0.000334 MB
        self.assertIn(
            "On disk size (uncompressed artifact size) 0.0003814697265625 MB is higher than the maximum allowed 0.0003337860107421875 MB by the gate !",
            error_message,
        )

    def test_get_image_url(self):
        # Test basic agent image
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate._get_image_url(), "registry.ddbuild.io/ci/datadog-agent/agent:v71580015-668844-7-amd64")

        # Test Windows 2022 servercore
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_windows_2022_servercore_amd64",
            {"max_on_wire_size": 100, "max_on_disk_size": 100},
            MagicMock(),
        )
        self.assertEqual(
            gate._get_image_url(),
            "registry.ddbuild.io/ci/datadog-agent/agent:v71580015-668844-7-winltsc2022-servercore-amd64",
        )

        # Test Windows 1809 servercore arm64
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_windows_1809_servercore_arm64",
            {"max_on_wire_size": 100, "max_on_disk_size": 100},
            MagicMock(),
        )
        self.assertEqual(
            gate._get_image_url(),
            "registry.ddbuild.io/ci/datadog-agent/agent:v71580015-668844-7-win1809-servercore-arm64",
        )

        # Test nightly builds
        with patch.dict('os.environ', {"BUCKET_BRANCH": "nightly"}):
            gate = StaticQualityGateDocker(
                "static_quality_gate_docker_agent_amd64",
                {"max_on_wire_size": 100, "max_on_disk_size": 100},
                MagicMock(),
            )
            self.assertEqual(
                gate._get_image_url(), "registry.ddbuild.io/ci/datadog-agent/agent-nightly:v71580015-668844-7-amd64"
            )

        # Test cluster-agent flavor
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_cluster_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(
            gate._get_image_url(), "registry.ddbuild.io/ci/datadog-agent/cluster-agent:v71580015-668844-amd64"
        )

        # Test dogstatsd flavor
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_dogstatsd_arm64",
            {"max_on_wire_size": 100, "max_on_disk_size": 100},
            MagicMock(),
        )
        self.assertEqual(gate._get_image_url(), "registry.ddbuild.io/ci/datadog-agent/dogstatsd:v71580015-668844-arm64")

        # Test cws_instrumentation flavor
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_cws_instrumentation_amd64",
            {"max_on_wire_size": 100, "max_on_disk_size": 100},
            MagicMock(),
        )
        self.assertEqual(
            gate._get_image_url(), "registry.ddbuild.io/ci/datadog-agent/cws-instrumentation:v71580015-668844-amd64"
        )

        # Test JMX images
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_jmx_amd64",
            {"max_on_wire_size": 100, "max_on_disk_size": 100},
            MagicMock(),
        )
        self.assertEqual(
            gate._get_image_url(), "registry.ddbuild.io/ci/datadog-agent/agent:v71580015-668844-7-jmx-amd64"
        )

        # Test Windows
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_windows_2022_amd64",
            {"max_on_wire_size": 100, "max_on_disk_size": 100},
            MagicMock(),
        )
        self.assertEqual(
            gate._get_image_url(), "registry.ddbuild.io/ci/datadog-agent/agent:v71580015-668844-7-winltsc2022-amd64"
        )

        # Test Windows 1809
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_windows_1809_amd64",
            {"max_on_wire_size": 100, "max_on_disk_size": 100},
            MagicMock(),
        )
        self.assertEqual(
            gate._get_image_url(), "registry.ddbuild.io/ci/datadog-agent/agent:v71580015-668844-7-win1809-amd64"
        )

        # Test JMX + Windows combination
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_jmx_windows_2022_servercore_amd64",
            {"max_on_wire_size": 100, "max_on_disk_size": 100},
            MagicMock(),
        )
        self.assertEqual(
            gate._get_image_url(),
            "registry.ddbuild.io/ci/datadog-agent/agent:v71580015-668844-7-jmx-winltsc2022-servercore-amd64",
        )

        # Test nightly with other flavors
        with patch.dict('os.environ', {"BUCKET_BRANCH": "nightly"}):
            gate = StaticQualityGateDocker(
                "static_quality_gate_docker_cluster_amd64",
                {"max_on_wire_size": 100, "max_on_disk_size": 100},
                MagicMock(),
            )
            self.assertEqual(
                gate._get_image_url(),
                "registry.ddbuild.io/ci/datadog-agent/cluster-agent-nightly:v71580015-668844-amd64",
            )

    def test_get_image_url_error_cases(self):
        # Test unknown flavor
        with self.assertRaises(ValueError) as context:
            gate = StaticQualityGateDocker(
                "static_quality_gate_docker_unknown_flavor_amd64",
                {"max_on_wire_size": 100, "max_on_disk_size": 100},
                MagicMock(),
            )
            gate._get_image_url()
        self.assertIn("Unknown docker image flavor for gate", str(context.exception))

        # Test missing CI_PIPELINE_ID
        with patch.dict('os.environ', {"CI_PIPELINE_ID": ""}):
            gate = StaticQualityGateDocker(
                "static_quality_gate_docker_agent_amd64",
                {"max_on_wire_size": 100, "max_on_disk_size": 100},
                MagicMock(),
            )
            with self.assertRaises(StaticQualityGateFailed) as context:
                gate._get_image_url()
            self.assertIn("Missing CI_PIPELINE_ID, CI_COMMIT_SHORT_SHA", str(context.exception))

        # Test missing CI_COMMIT_SHORT_SHA
        with patch.dict('os.environ', {"CI_COMMIT_SHORT_SHA": ""}):
            gate = StaticQualityGateDocker(
                "static_quality_gate_docker_agent_amd64",
                {"max_on_wire_size": 100, "max_on_disk_size": 100},
                MagicMock(),
            )
            with self.assertRaises(StaticQualityGateFailed) as context:
                gate._get_image_url()
            self.assertIn("Missing CI_PIPELINE_ID, CI_COMMIT_SHORT_SHA", str(context.exception))

    def test_execute_gate_package_success(self):
        """Test successful execution of execute_gate for StaticQualityGatePackage"""
        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_deb_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 350}, MagicMock()
        )

        # Mock the internal methods that execute_gate calls
        gate._measure_on_disk_and_on_wire_size = MagicMock()
        gate.check_artifact_size = MagicMock()
        gate.print_results = MagicMock()
        gate.artifact_path = "/opt/datadog-agent/test-package.deb"

        # Capture printed output
        with patch('builtins.print') as mock_print:
            gate.execute_gate()

        # Verify that all internal methods were called
        gate._measure_on_disk_and_on_wire_size.assert_called_once()
        gate.check_artifact_size.assert_called_once()
        gate.print_results.assert_called_once()

        # Verify the expected print statements
        # Check that print was called the expected number of times
        self.assertEqual(mock_print.call_count, 4)

        # Verify the execution message with enhanced formatting
        mock_print.assert_any_call("\n🔍 Checking Agent DEB (AMD64)")
        mock_print.assert_any_call("📄 Artifact: test-package.deb")
        mock_print.assert_any_call("\x1b[92m✅ Agent DEB (AMD64) PASSED\x1b[0m")
        mock_print.assert_any_call("-" * 80)
