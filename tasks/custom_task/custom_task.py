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

DD_INVOKE_LOGS_PATH = "/tmp/dd_invoke.log"


def log_invoke_task(name: str, module: str, task_datetime: str, duration: float, task_result: str) -> None:
    """
    Logs the task information to the DD_INVOKE_LOGS_PATH file.
    This should be uploaded to Datadog's backend with a correct Log Agent configuration:
    ```
    logs:
    - type: file
        path: "/tmp/dd_invoke.log"
        service: "dd_invoke_logs"
        source: "invoke"
    ```
    """
    logging.basicConfig(filename=DD_INVOKE_LOGS_PATH, level=logging.INFO, format='%(message)s')
    user = getuser()
    running_mode = "pre_commit" if os.environ.get("PRE_COMMIT", 0) == "1" else "manual"
    task_info = {
        "name": name,
        "module": module,
        "running_mode": running_mode,
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
        self.datetime = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        self.start = None
        self.end = None

    def __enter__(self):
        self.start = perf_counter()

    def __exit__(self, exc_type, exc_value, exc_traceback):
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
                name=name, module=module, task_datetime=self.datetime, duration=duration, task_result=task_result
            )
        except Exception as e:
            print(
                f"Warning: couldn't log the invoke task in the InvokeLogger context manager (error: {e})",
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
