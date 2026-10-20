import unittest
from unittest.mock import MagicMock, patch

from tasks.fuzz import fuzz


class TestFuzz(unittest.TestCase):
    @patch("tasks.fuzz.schema_codegen")
    @patch("tasks.fuzz.search_fuzz_tests")
    def test_limits_discovery_to_requested_package(self, search, _):
        target = "pkg/rshell/builtins/tests/head"
        search.return_value = [(target, "FuzzHeadLines")]
        ctx = MagicMock()

        fuzz.body(ctx, targets=target, fuzztime="1s")

        search.assert_called_once_with(target)
        ctx.cd.assert_called_once_with(target)
        command = ctx.run.call_args.args[0]
        self.assertIn("-fuzz=FuzzHeadLines$", command)
        self.assertIn("-fuzztime=1s", command)
