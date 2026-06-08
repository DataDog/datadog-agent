import unittest
from unittest.mock import patch

from invoke import MockContext

from tasks.dogstatsd import DOGSTATSD_CONFIG_OUTPUT, build
from tasks.schema.template import CORE_SCHEMA_FILE


class TestDogstatsdBuild(unittest.TestCase):
    @patch("tasks.dogstatsd.refresh_assets")
    @patch("tasks.dogstatsd.generate_template")
    @patch("tasks.dogstatsd.go_build")
    @patch("tasks.dogstatsd.get_build_flags")
    @patch("tasks.dogstatsd.sys")
    def test_build_renders_dogstatsd_template(self, sys_mod, gbf, _go, gen, _refresh):
        sys_mod.platform = "linux"
        gbf.return_value = ("ld", "gc", {})
        ctx = MockContext()

        build(ctx)

        gen.assert_called_once_with(
            CORE_SCHEMA_FILE,
            DOGSTATSD_CONFIG_OUTPUT,
            "dogstatsd",
            "linux",
        )


if __name__ == "__main__":
    unittest.main()
