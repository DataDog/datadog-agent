import unittest
import unittest.mock
from datetime import datetime

from invoke import Context, task

from tasks.custom_task.custom_task import log_invoke_task


@task
def test_task(_):
    """
    Dummy task for testing purposes.
    """
    return "Nice task"


test_task.__module__ = "my_module"


@task
def test_task_with_error(_):
    """
    Dummy task returning an error for testing purposes.
    """
    raise TypeError("Oh no this is not a good type !")


test_task_with_error.__module__ = "my_broken_module"


class TestInvokeTaskCustomCall(unittest.TestCase):
    """
    Testing the __call__ method (overridden with custom__call__) log generation.
    """

    @unittest.mock.patch('tasks.custom_task.custom_task.datetime')
    @unittest.mock.patch('tasks.custom_task.custom_task.perf_counter', side_effect=[1, 1.0123456, 10, 15])
    @unittest.mock.patch('tasks.custom_task.custom_task.getuser', side_effect=['john.doe', 'alex.smith'])
    @unittest.mock.patch("tasks.custom_task.custom_task.logging.info")
    def test_custom__call__(self, mock_logging, _getuser, _perf_counter, mock_datetime):
        """
        Testing the __call__ method (overridden with custom__call__) log generation.
        """
        ctx = Context()
        mock_datetime.now.return_value = datetime(2024, 4, 29, 14, 5, 30, 367616)

        # Testing the context manager with a successful task.
        expected_test_log = {
            "name": "test_task",
            "module": "my_module",
            "datetime": "2024-04-29 14:05:30",
            "duration": 0.0123,
            "user": "john.doe",
            "result": None,
        }
        test_task.__call__(ctx)
        mock_logging.assert_called_once_with(expected_test_log)

        # Testing the context manager with a failing task.
        expected_test_error_log = {
            "name": "test_task_with_error",
            "module": "my_broken_module",
            "datetime": "2024-04-29 14:05:30",
            "duration": 5.0000,
            "user": "alex.smith",
            "result": unittest.mock.ANY,
        }
        with self.assertRaises(TypeError):
            test_task_with_error.__call__(ctx)

        ## Check that the logger was called with the expected arguments.
        mock_logging.assert_called_with(expected_test_error_log)

        ## Check that the traceback is returned in the log.
        expected_test_error_result = mock_logging.call_args_list[1].args[0]['result']
        self.assertIn('Traceback (most recent call last)', expected_test_error_result)
        self.assertIn('TypeError', expected_test_error_result)
        self.assertIn('Oh no this is not a good type !', expected_test_error_result)


class TestLogInvokeTask(unittest.TestCase):
    """
    Testing the log_invoke_task function.
    """

    @unittest.mock.patch('tasks.custom_task.custom_task.getuser', side_effect=['john.smith'])
    @unittest.mock.patch("tasks.custom_task.custom_task.logging.info")
    def test_log_invoke_task(self, mock_logging, _getuser):
        """
        Testing the log_invoke_task function.
        """
        log_invoke_task(
            name="tname", module="mname", task_datetime="2024-01-29 07:50:11", duration=3.1, task_result="result"
        )
        mock_logging.assert_called_once_with(
            {
                "name": "tname",
                "module": "mname",
                "datetime": "2024-01-29 07:50:11",
                "duration": 3.1,
                "user": "john.smith",
                "result": "result",
            }
        )
