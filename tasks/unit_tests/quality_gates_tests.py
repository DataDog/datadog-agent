import os
import unittest
from unittest.mock import ANY, MagicMock, patch

from invoke import Context, MockContext, Result

from tasks.quality_gates import display_pr_comment, generate_new_quality_gate_config
from tasks.static_quality_gates.lib.docker_agent_lib import calculate_image_on_disk_size
from tasks.static_quality_gates.lib.gates_lib import GateMetricHandler


class MockMetricHandler:
    def __init__(self, metrics):
        self.metrics = metrics
        self.total_size_saved = 0


class TestQualityGatesConfigUpdate(unittest.TestCase):
    def test_one_gate_update(self):
        with open("tasks/unit_tests/testdata/quality_gate_config_test.yml") as f:
            new_config, saved_amount = generate_new_quality_gate_config(
                f,
                MockMetricHandler(
                    {
                        "some_gate_high": {
                            "current_on_wire_size": 50000000,
                            "max_on_wire_size": 100000000,
                            "current_on_disk_size": 50000000,
                            "max_on_disk_size": 100000000,
                        },
                        "some_gate_low": {
                            "current_on_wire_size": 4000000,
                            "max_on_wire_size": 5000000,
                            "current_on_disk_size": 4000000,
                            "max_on_disk_size": 5000000,
                        },
                    }
                ),
            )
        assert new_config["some_gate_high"]["max_on_wire_size"] == "48.64 MiB", print(
            f"Expected 48.64 MiB got {new_config['some_gate_high']['max_on_wire_size']}"
        )
        assert new_config["some_gate_high"]["max_on_disk_size"] == "48.64 MiB", print(
            f"Expected 48.64 MiB got {new_config['some_gate_high']['max_on_disk_size']}"
        )
        assert new_config["some_gate_low"]["max_on_wire_size"] == "4.77 MiB", print(
            f"Expected 4.77 MiB got {new_config['some_gate_low']['max_on_wire_size']}"
        )
        assert new_config["some_gate_low"]["max_on_disk_size"] == "4.77 MiB", print(
            f"Expected 4.77 MiB got {new_config['some_gate_low']['max_on_disk_size']}"
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
        )
        pr_commenter_mock.assert_called_once()
        pr_commenter_mock.assert_called_with(
            ANY,
            title='Static quality checks',
            body='✅ Please find below the results from static quality gates\n\n\n<details>\n<summary>Successful checks</summary>\n\n### Info\n\n|Result|Quality gate|On disk size|On disk size limit|On wire size|On wire size limit|\n|----|----|----|----|----|----|\n|✅|gateA|10MiB|10MiB|10MiB|10MiB|\n|✅|gateB|10MiB|10MiB|10MiB|10MiB|\n\n</details>\n',
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
        )
        pr_commenter_mock.assert_called_once()
        pr_commenter_mock.assert_called_with(
            ANY,
            title='Static quality checks',
            body='❌ Please find below the results from static quality gates\n### Error\n\n|Result|Quality gate|On disk size|On disk size limit|On wire size|On wire size limit|\n|----|----|----|----|----|----|\n|❌|gateA|10MiB|10MiB|10MiB|10MiB|\n|❌|gateB|10MiB|10MiB|10MiB|10MiB|\n<details>\n<summary>Gate failure full details</summary>\n\n|Quality gate|Error type|Error message|\n|----|---|--------|\n|gateA|AssertionError|some_msg_A|\n|gateB|AssertionError|some_msg_B|\n\n</details>\n\nStatic quality gates prevent the PR to merge! You can check the static quality gates [confluence page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4805854687/Static+Quality+Gates) for guidance. We also have a [toolbox page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4887448722/Static+Quality+Gates+Toolbox) available to list tools useful to debug the size increase.\n\n\n',
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
        )
        pr_commenter_mock.assert_called_once()
        pr_commenter_mock.assert_called_with(
            ANY,
            title='Static quality checks',
            body='❌ Please find below the results from static quality gates\n### Error\n\n|Result|Quality gate|On disk size|On disk size limit|On wire size|On wire size limit|\n|----|----|----|----|----|----|\n|❌|gateA|10MiB|10MiB|10MiB|10MiB|\n<details>\n<summary>Gate failure full details</summary>\n\n|Quality gate|Error type|Error message|\n|----|---|--------|\n|gateA|AssertionError|some_msg_A|\n\n</details>\n\nStatic quality gates prevent the PR to merge! You can check the static quality gates [confluence page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4805854687/Static+Quality+Gates) for guidance. We also have a [toolbox page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4887448722/Static+Quality+Gates+Toolbox) available to list tools useful to debug the size increase.\n\n\n<details>\n<summary>Successful checks</summary>\n\n### Info\n\n|Result|Quality gate|On disk size|On disk size limit|On wire size|On wire size limit|\n|----|----|----|----|----|----|\n|✅|gateB|10MiB|10MiB|10MiB|10MiB|\n\n</details>\n',
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
        )
        pr_commenter_mock.assert_called_once()
        pr_commenter_mock.assert_called_with(
            ANY,
            title='Static quality checks',
            body='❌ Please find below the results from static quality gates\n### Error\n\n|Result|Quality gate|On disk size|On disk size limit|On wire size|On wire size limit|\n|----|----|----|----|----|----|\n|❌|gateA|DataNotFound|DataNotFound|DataNotFound|DataNotFound|\n<details>\n<summary>Gate failure full details</summary>\n\n|Quality gate|Error type|Error message|\n|----|---|--------|\n|gateA|AssertionError|some_msg_A|\n\n</details>\n\nStatic quality gates prevent the PR to merge! You can check the static quality gates [confluence page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4805854687/Static+Quality+Gates) for guidance. We also have a [toolbox page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4887448722/Static+Quality+Gates+Toolbox) available to list tools useful to debug the size increase.\n\n\n',
        )


class DynamicMockContext:
    def __init__(self, actual_context, mock_context):
        self.actual_context = actual_context
        self.mock_context = mock_context

    def run(self, *args, **kwargs):
        try:
            return self.mock_context.run(*args, **kwargs)
        except NotImplementedError:
            return self.actual_context.run(*args, **kwargs)


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

    def test_compute_image_size(self):
        actualContext = Context()
        c = MockContext(
            run={
                'crane pull some_url output.tar': Result('Done'),
                "tar -tvf output.tar | awk -F' ' '{print $3; print $6}'": Result(
                    "3\nsome_metadata.json\n9728\nsome_archive.tar.gz"
                ),
            }
        )
        cwd = os.getcwd()
        os.chdir(os.path.abspath('./tasks/unit_tests/testdata/fake_agent_image/with_tar_gz_archive/'))
        context = DynamicMockContext(actual_context=actualContext, mock_context=c)
        calculated_size = calculate_image_on_disk_size(context, "some_url")
        os.chdir(cwd)
        assert calculated_size == 5861

    def test_metadata_only(self):
        actualContext = Context()
        c = MockContext(
            run={
                'crane pull some_url output.tar': Result('Done'),
                "tar -tvf output.tar | awk -F' ' '{print $3; print $6}'": Result("3\nsome_metadata.json"),
            }
        )
        cwd = os.getcwd()
        os.chdir(os.path.abspath('./tasks/unit_tests/testdata/fake_agent_image/without_tar_gz_archive/'))
        context = DynamicMockContext(actual_context=actualContext, mock_context=c)
        calculated_size = calculate_image_on_disk_size(context, "some_url")
        os.chdir(cwd)
        assert calculated_size == 3
