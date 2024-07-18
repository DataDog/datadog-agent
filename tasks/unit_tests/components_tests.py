import os
import shutil
import tempfile
import unittest
from pathlib import Path

from tasks import components


class TestComponents(unittest.TestCase):
    def setUp(self):
        # Create a temporary directory of the source directory to allow
        # tests to modify the source files
        # TODO: Add tests that modify source files to ensure that errors work
        self.tmpdir = tempfile.mkdtemp()
        shutil.copytree('tasks/unit_tests/testdata/components_src/comp', os.path.join(self.tmpdir, 'comp'))
        # Preserve currenty directory, change to temp directory
        self.origDir = os.getcwd()
        os.chdir(self.tmpdir)

    def tearDown(self):
        if self.tmpdir:
            shutil.rmtree(self.tmpdir)
        os.chdir(self.origDir)

    def test_find_team(self):
        content = ['// my file', '// team: agent-shared-components', '// file starts here']
        teamname = components.find_team(content)
        self.assertEqual(teamname, 'agent-shared-components')

    def test_get_components_and_bundles(self):
        results = components.get_components_and_bundles()
        bundles = results[0]
        comps = results[1]
        ok = results[2]
        self.assertEqual([], bundles)
        self.assertEqual(3, len(comps))
        self.assertEqual('comp/classic', comps[0].path)
        self.assertEqual('comp/multiple', comps[1].path)
        self.assertEqual('comp/newstyle', comps[2].path)
        self.assertEqual(ok, False)

        # Add the classic component (used by tests) to the allowlist
        # To validate that calling get_components_and_bundles does not return an error
        classicComp = 'comp/classic/classicimpl'
        if classicComp not in components.components_classic_style:
            components.components_classic_style.append(classicComp)

        results = components.get_components_and_bundles()
        bundles = results[0]
        comps = results[1]
        ok = results[2]
        self.assertEqual([], bundles)
        self.assertEqual(3, len(comps))
        self.assertEqual(ok, True)

        components.components_classic_style.remove(classicComp)

    def test_locate_root(self):
        root = components.locate_root(Path('comp/classic'))
        self.assertEqual(1, root.version)

        root = components.locate_root(Path('comp/multiple'))
        self.assertEqual(2, root.version)

        root = components.locate_root(Path('comp/newstyle'))
        self.assertEqual(2, root.version)

    # TODO: more tests
