import unittest

from compliance.go_licenses import classify, is_excepted


class IsExceptedTest(unittest.TestCase):
    def test_subtree_pattern(self):
        self.assertTrue(is_excepted("golang.org/x/mod/semver", ["golang.org/x/mod/..."]))
        self.assertTrue(is_excepted("golang.org/x/mod", ["golang.org/x/mod/..."]))

    def test_subtree_pattern_is_not_a_string_prefix(self):
        self.assertFalse(is_excepted("golang.org/x/modfile", ["golang.org/x/mod/..."]))

    def test_exact_pattern(self):
        self.assertTrue(is_excepted("github.com/stretchr/testify", ["github.com/stretchr/testify"]))
        self.assertFalse(is_excepted("github.com/stretchr/testify/assert", ["github.com/stretchr/testify"]))


class ClassifyTest(unittest.TestCase):
    licenses = {
        "golang.org/x/time/rate": "BSD-3-Clause",
        "github.com/foo/bar": "MIT",
    }

    def test_matches_and_dedups(self):
        packages = [
            {"importpath": "golang.org/x/time/rate", "label": "@a//rate", "purl": "pkg:golang/golang.org/x/time@v0.16.0"},
            {"importpath": "golang.org/x/time/rate", "label": "@a//rate", "purl": "pkg:golang/golang.org/x/time@v0.16.0"},
            {"importpath": "github.com/foo/bar", "label": "@b//:bar", "purl": ""},
        ]
        matched, unmatched = classify(packages, self.licenses, [])
        self.assertEqual(
            matched,
            [
                ("github.com/foo/bar", "", "MIT"),
                ("golang.org/x/time/rate", "pkg:golang/golang.org/x/time@v0.16.0", "BSD-3-Clause"),
            ],
        )
        self.assertEqual(unmatched, [])

    def test_exceptions_are_dropped(self):
        packages = [{"importpath": "golang.org/x/mod/semver", "label": "@m//semver", "purl": ""}]
        matched, unmatched = classify(packages, self.licenses, ["golang.org/x/mod/..."])
        self.assertEqual(matched, [])
        self.assertEqual(unmatched, [])

    def test_unknown_package_is_unmatched(self):
        packages = [{"importpath": "example.com/unknown", "label": "@u//:unknown", "purl": ""}]
        matched, unmatched = classify(packages, self.licenses, [])
        self.assertEqual(matched, [])
        self.assertEqual(unmatched, [("example.com/unknown", "@u//:unknown")])


if __name__ == "__main__":
    unittest.main()
