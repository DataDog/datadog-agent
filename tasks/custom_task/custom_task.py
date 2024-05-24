"""
This module defines the InvokeLogger context manager.
It logs the invoke task information to the DD_INVOKE_LOGS_PATH.
This will then be uploaded to Datadog's backend with a correct Log Agent configuration.
"""

import logging
import os
import sys
import traceback
from datetime import datetime
from getpass import getuser
from time import perf_counter

from invoke import Context

from tasks.libs.common.color import color_message
from tasks.libs.common.utils import running_in_ci

DD_INVOKE_LOGS_FILE = "dd_invoke.log"
WIN_TEMP_FOLDER = "C:\\Windows\\Temp"
UNIX_TEMP_FOLDER = "/tmp"


def get_dd_invoke_logs_path() -> str:
    """
    Get the path to the invoke tasks log file.
    On Windows the default path is C:\\Windows\\Temp\\dd_invoke.log
    On Linux & MacOS the default path is /tmp/dd_invoke.log
    """
    temp_folder = WIN_TEMP_FOLDER if sys.platform == 'win32' else UNIX_TEMP_FOLDER
    return os.path.join(temp_folder, DD_INVOKE_LOGS_FILE)


def get_running_modes() -> list[str]:
    """
    List the running modes of the task.
    If the task is run via pre-commit -> "pre_commit"
    If the task is run via unittest   -> "invoke_unit_tests"
    If the task is run in the ci      -> "ci"
    Neither pre-commit nor ci         -> "manual"
    """
    # This will catch when devs are running the unit tests with the unittest module directly.
    # When running the unit tests with the invoke command, the INVOKE_UNIT_TESTS env variable is set.
    is_running_ut = "unittest" in " ".join(sys.argv)
    running_modes = {
        "pre_commit": os.environ.get("PRE_COMMIT", 0) == "1",
        "invoke_unit_tests": is_running_ut or os.environ.get("INVOKE_UNIT_TESTS", 0) == "1",
        "ci": running_in_ci(),
        "pyapp": os.environ.get("PYAPP") == "1",
    }
    running_modes["manual"] = not (running_modes["pre_commit"] or running_modes["ci"])
    return [mode for mode, is_running in running_modes.items() if is_running]


def log_invoke_task(
    log_path: str, name: str, module: str, task_datetime: str, duration: float, task_result: str
) -> None:
    """
    Logs the task information to the dd_invoke_logs_path file.
    This should be uploaded to Datadog's backend with a correct Log Agent configuration. E.g on MacOS:
    ```
    logs:
    - type: file
        path: "/tmp/dd_invoke.log"
        service: "dd_invoke_logs"
        source: "invoke"
    ```
    """
    logging.basicConfig(filename=log_path, level=logging.INFO, format='%(message)s')
    user = getuser()
    running_modes = get_running_modes()
    task_info = {
        "name": name,
        "module": module,
        "running_modes": running_modes,
        "datetime": task_datetime,
        "duration": duration,
        "user": user,
        "result": task_result,
    }
    logging.info(task_info)


class InvokeLogger:
    """
    Context manager to log an invoke task run information.
    """

    def __init__(self, task) -> None:
        self.task = task
        self.log_path = get_dd_invoke_logs_path()
        self.datetime = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        self.start = None
        self.end = None

    def __enter__(self):
        self.start = perf_counter()

    def __exit__(self, exc_type, exc_value, exc_traceback):
        # If the log_path is not set, don't log the task information.
        # A warning was already raised in the get_dd_invoke_logs_path function.
        if self.log_path == "":
            return
        # Avoid disturbing the smooth running of the task by wrapping the logging in a try-except block.
        try:
            self.end = perf_counter()
            duration = round(self.end - self.start, 4)
            name = self.task.__name__
            module = self.task.__module__.replace("tasks.", "")
            task_result = (
                None if exc_type is None else "".join(traceback.format_exception(exc_type, exc_value, exc_traceback))
            )
            log_invoke_task(
                log_path=self.log_path,
                name=name,
                module=module,
                task_datetime=self.datetime,
                duration=duration,
                task_result=task_result,
            )
        except Exception as e:
            print(
                color_message(
                    message=f"Warning: couldn't log the invoke task in the InvokeLogger context manager (error: {e})",
                    color="orange",
                ),
                file=sys.stderr,
            )


def custom__call__(self, *args, **kwargs):
    """
    Custom __call__ method for the Task class.
    The code was adapted from the invoke 2.2.0 library's Task class.
    """

    ## LEGACY INVOKE LIB CODE ##
    # Guard against calling tasks with no context.
    if not isinstance(args[0], Context):
        err = "Task expected a Context as its first arg, got {} instead!"
        # TODO: raise a custom subclass _of_ TypeError instead
        raise TypeError(err.format(type(args[0])))

    ## DATADOG INVOKE LOGGER CODE ##
    with InvokeLogger(self):
        result = self.body(*args, **kwargs)

    ## LEGACY INVOKE LIB CODE ##
    self.times_called += 1  # noqa
    return result
