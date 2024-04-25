import inspect
from invoke import Task
import invoke
import unittest

expected_content = """    def __call__(self, *args: Any, **kwargs: Any) -> T:
        # Guard against calling tasks with no context.
        if not isinstance(args[0], Context):
            err = "Task expected a Context as its first arg, got {} instead!"
            # TODO: raise a custom subclass _of_ TypeError instead
            raise TypeError(err.format(type(args[0])))
        result = self.body(*args, **kwargs)
        self.times_called += 1
        return result
"""


class TestTaskCall(unittest.TestCase):
    def test_task_call_content(self):
        self.assertEqual(inspect.getsource(Task.__call__), expected_content)

    def test_invoke_version(self):
        self.assertEqual(invoke.__version__, '2.2.0')
