import os
import unittest
from unittest.mock import ANY, MagicMock, patch

from invoke import Context, MockContext, Result

from tasks.quality_gates import display_pr_comment
from tasks.static_quality_gates.lib.gates_lib import GateMetricHandler
from tasks.static_quality_gates.static_quality_gate_docker_agent_amd64 import calculate_image_on_disk_size


class TestQualityGatesPrMessage(unittest.TestCase):
    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch(
        "tasks.static_quality_gates.lib.gates_lib.GateMetricHandler.get_formated_metric",
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
            title='Static quality checks ✅',
            body='Please find below the results from static quality gates\n\n\n### Info\n\n|Result|Quality gate|On disk size|On disk size limit|On wire size|On wire size limit|\n|----|----|----|----|----|----|\n|✅|gateA|10MiB|10MiB|10MiB|10MiB|\n|✅|gateB|10MiB|10MiB|10MiB|10MiB|\n',
        )

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch(
        "tasks.static_quality_gates.lib.gates_lib.GateMetricHandler.get_formated_metric",
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
            title='Static quality checks ❌',
            body='Please find below the results from static quality gates\n### Error\n\n|Result|Quality gate|On disk size|On disk size limit|On wire size|On wire size limit|\n|----|----|----|----|----|----|\n|❌|gateA|10MiB|10MiB|10MiB|10MiB|\n|❌|gateB|10MiB|10MiB|10MiB|10MiB|\n<details>\n<summary>Gate failure full details</summary>\n\n|Quality gate|Error type|Error message|\n|----|---|--------|\n|gateA|AssertionError|some_msg_A|\n|gateB|AssertionError|some_msg_B|\n\n</details>\n\n\n',
        )

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch(
        "tasks.static_quality_gates.lib.gates_lib.GateMetricHandler.get_formated_metric",
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
            title='Static quality checks ❌',
            body='Please find below the results from static quality gates\n### Error\n\n|Result|Quality gate|On disk size|On disk size limit|On wire size|On wire size limit|\n|----|----|----|----|----|----|\n|❌|gateA|10MiB|10MiB|10MiB|10MiB|\n<details>\n<summary>Gate failure full details</summary>\n\n|Quality gate|Error type|Error message|\n|----|---|--------|\n|gateA|AssertionError|some_msg_A|\n\n</details>\n\n\n### Info\n\n|Result|Quality gate|On disk size|On disk size limit|On wire size|On wire size limit|\n|----|----|----|----|----|----|\n|✅|gateB|10MiB|10MiB|10MiB|10MiB|\n',
        )

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch(
        "tasks.static_quality_gates.lib.gates_lib.GateMetricHandler.get_formated_metric",
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
            title='Static quality checks ❌',
            body='Please find below the results from static quality gates\n### Error\n\n|Result|Quality gate|On disk size|On disk size limit|On wire size|On wire size limit|\n|----|----|----|----|----|----|\n|❌|gateA|DataNotFound|DataNotFound|DataNotFound|DataNotFound|\n<details>\n<summary>Gate failure full details</summary>\n\n|Quality gate|Error type|Error message|\n|----|---|--------|\n|gateA|AssertionError|some_msg_A|\n\n</details>\n\n\n',
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
            os.remove('rm ./tasks/unit_tests/testdata/fake_agent_image/with_tar_gz_archive/some_archive.tar.gz')
            os.remove('rm ./tasks/unit_tests/testdata/fake_agent_image/with_tar_gz_archive/some_metadata.json')
        except OSError:
            pass
        try:
            os.remove('rm ./tasks/unit_tests/testdata/fake_agent_image/without_tar_gz_archive/some_metadata.json')
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
