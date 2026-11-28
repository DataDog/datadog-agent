import unittest
from unittest.mock import patch

from invoke import MockContext

from tasks.flavor import FipsBackend
from tasks.secret_generic_connector import build


@patch("tasks.secret_generic_connector.get_version", return_value="7.0.0")
@patch("tasks.secret_generic_connector.go_build")
class TestSecretGenericConnectorBuild(unittest.TestCase):
    def _ldflags(self, go_build, goos, **kwargs):
        with (
            patch("tasks.secret_generic_connector.get_build_flags", return_value=("", "", {"GOOS": goos})),
            patch("tasks.secret_generic_connector.sys.platform", goos),
        ):
            build(MockContext(), **kwargs)
        return go_build.call_args.kwargs["ldflags"].split()

    def test_linux_msgo_rpath_defaults_to_embedded_lib(self, go_build, _):
        ldflags = self._ldflags(go_build, "linux", fips_backend=FipsBackend.msgo)
        self.assertIn("-r /opt/datadog-agent/embedded/lib", " ".join(ldflags))

    def test_linux_msgo_rpath_follows_install_path(self, go_build, _):
        ldflags = self._ldflags(go_build, "linux", fips_backend=FipsBackend.msgo, install_path="/custom")
        self.assertIn("-r /custom/embedded/lib", " ".join(ldflags))

    def test_no_rpath_for_other_builds(self, go_build, _):
        for goos, fips_backend in [
            ("linux", FipsBackend.none),
            ("linux", FipsBackend.boringcrypto),
            ("windows", FipsBackend.msgo),
        ]:
            with self.subTest(goos=goos, fips_backend=fips_backend):
                self.assertNotIn("-r", self._ldflags(go_build, goos, fips_backend=fips_backend))


if __name__ == "__main__":
    unittest.main()
