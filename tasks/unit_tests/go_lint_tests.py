import os
import unittest
from unittest.mock import patch

from tasks import go


def _go_quoted_split(s):
    """Same as Go's cmd/internal/quoted.Split, which cmd/go applies to GOFLAGS and to the -gcflags value."""
    fields = []
    while s:
        s = s.lstrip(" \t\n\r")
        if not s:
            break
        if s[0] in "'\"":
            end = s.index(s[0], 1)
            fields.append(s[1:end])
            s = s[end + 1 :]
        else:
            end = next((i for i, c in enumerate(s) if c in " \t\n\r"), len(s))
            fields.append(s[:end])
            s = s[end:]
    return fields


def _compiler_flags(goflag):
    """Returns the compiler flags cmd/go gets from a GOFLAGS entry `-gcflags=all=<flags>`."""
    [field] = _go_quoted_split(goflag)
    prefix = "-gcflags=all="
    assert field.startswith(prefix), field
    return _go_quoted_split(field[len(prefix) :])


class TestGcflagsGoflag(unittest.TestCase):
    def test_reaches_the_compiler_as_one_flag(self):
        for path in ("/tmp/cache/x.o", "/home/o'brien/x.o", '/tmp/a"b/x.o', "/tmp/dir with space/x.o"):
            with self.subTest(path=path):
                flag = f"-linkobj={path}"
                self.assertEqual(_compiler_flags(go._gcflags_goflag(flag)), [flag])

    def test_whitespace_and_quote_cannot_be_expressed(self):
        self.assertIsNone(go._gcflags_goflag("-linkobj=/home/O'Brien temp/x.o"))
        self.assertIsNone(go._gcflags_goflag('-linkobj=/tmp/a "b"/x.o'))


class TestUserCacheDir(unittest.TestCase):
    def test_linux_prefers_xdg_cache_home(self):
        with patch.object(go.sys, "platform", "linux"), patch.dict(os.environ, {"XDG_CACHE_HOME": "/xdg"}):
            self.assertEqual(go._user_cache_dir(), "/xdg")

    def test_linux_defaults_to_home_cache(self):
        env = {k: v for k, v in os.environ.items() if k != "XDG_CACHE_HOME"} | {"HOME": "/home/me"}
        with patch.object(go.sys, "platform", "linux"), patch.dict(os.environ, env, clear=True):
            self.assertEqual(go._user_cache_dir(), "/home/me/.cache")
