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
        self.tmpdir = tempfile.mkdtemp()
        self.origDir = os.getcwd()
        self.reset_component_src_in_tmpdir()
        # Preserve currenty directory, change to temp directory
        os.chdir(self.tmpdir)
        classicComp = 'comp/classic/classicimpl'
        if classicComp not in components.components_classic_style:
            components.components_classic_style.append(classicComp)

    def reset_component_src_in_tmpdir(self):
        shutil.copytree(
            os.path.join(self.origDir, 'tasks/unit_tests/testdata/components_src/comp'),
            os.path.join(self.tmpdir, 'comp'),
            dirs_exist_ok=True,
        )

    def tearDown(self):
        if self.tmpdir:
            shutil.rmtree(self.tmpdir)
        os.chdir(self.origDir)
        classicComp = 'comp/classic/classicimpl'
        components.components_classic_style.remove(classicComp)

    def test_find_team(self):
        content = ['// my file', '// team: agent-shared-components', '// file starts here']
        teamname = components.find_team(content)
        self.assertEqual(teamname, 'agent-shared-components')

    def test_get_components_and_bundles(self):
        comps, bundles = components.get_components_and_bundles()
        self.assertEqual(1, len(bundles))
        self.assertEqual('comp/group', bundles[0].path)
        self.assertEqual(1, len(bundles[0].components))
        self.assertEqual('comp/group/inbundle', bundles[0].components[0].path)
        self.assertEqual(4, len(comps))
        self.assertEqual('comp/classic', comps[0].path)
        self.assertEqual('comp/group/inbundle', comps[1].path)
        self.assertEqual('comp/multiple', comps[2].path)
        self.assertEqual('comp/newstyle', comps[3].path)

    def test_locate_root(self):
        root = components.locate_component_def(Path('comp/classic'))
        self.assertEqual(1, root.version)

        root = components.locate_component_def(Path('comp/multiple'))
        self.assertEqual(2, root.version)

        root = components.locate_component_def(Path('comp/newstyle'))
        self.assertEqual(2, root.version)

    def test_validate_bundles(self):
        _, bundles = components.get_components_and_bundles()
        errs = components.validate_bundles(bundles)
        self.assertEqual(0, len(errs))

        # Lint error because bundle defines an interface
        filename = os.path.join(bundles[0].path, 'bundle.go')
        append_line(filename, 'type Component interface{}')

        _, bundles = components.get_components_and_bundles()
        errs = components.validate_bundles(bundles)
        self.assertEqual(1, len(errs))
        self.assertIn('Component interface', errs[0])

        # No lint errors with original source
        self.reset_component_src_in_tmpdir()

        _, bundles = components.get_components_and_bundles()
        errs = components.validate_bundles(bundles)
        self.assertEqual(0, len(errs))

        # Lint error because team owner is missing
        remove_line(filename, '// team: agent-shared-components')

        _, bundles = components.get_components_and_bundles()
        errs = components.validate_bundles(bundles)
        self.assertEqual(1, len(errs))
        self.assertIn('team owner', errs[0])

    def test_validate_component_definition(self):
        comps, _ = components.get_components_and_bundles()
        errs = components.validate_components(comps)
        self.assertEqual(0, len(errs))

        # Lint error because team owner is missing
        filename = os.path.join(comps[3].path, 'def/component.go')
        remove_line(filename, '// team: agent-shared-components')

        comps, _ = components.get_components_and_bundles()
        errs = components.validate_components(comps)
        self.assertEqual(1, len(errs))
        self.assertIn('team owner', errs[0])

        # Lint error because of wrong package name
        self.reset_component_src_in_tmpdir()

        filename = os.path.join(comps[3].path, 'def/component.go')
        replace_line(filename, 'package newstyle', 'package def')

        comps, _ = components.get_components_and_bundles()
        errs = components.validate_components(comps)
        self.assertEqual(1, len(errs))
        self.assertIn('wrong package', errs[0])

        # Lint error because def/component.go doesn't define Component interface
        self.reset_component_src_in_tmpdir()

        filename = os.path.join(comps[3].path, 'def/component.go')
        remove_line(filename, 'type Component interface{}')

        comps, _ = components.get_components_and_bundles()
        errs = components.validate_components(comps)
        self.assertEqual(1, len(errs))
        self.assertIn('does not define', errs[0])

        # Lint error because def/component.go should not contain Mock interface
        self.reset_component_src_in_tmpdir()

        filename = os.path.join(comps[3].path, 'def/component.go')
        append_line(filename, 'type Mock interface{}')

        comps, _ = components.get_components_and_bundles()
        errs = components.validate_components(comps)
        self.assertEqual(1, len(errs))
        self.assertIn('separate implementation', errs[0])

    def test_validate_component_fx(self):
        comps, _ = components.get_components_and_bundles()
        errs = components.validate_components(comps)
        self.assertEqual(0, len(errs))

        # Lint error because fx/fx.go must define package fx or <component>fx
        filename = os.path.join(comps[3].path, 'fx/fx.go')
        replace_line(filename, 'package newstylefx', 'package newstyle')

        comps, _ = components.get_components_and_bundles()
        errs = components.validate_components(comps)
        self.assertEqual(1, len(errs))
        self.assertIn('wrong package name', errs[0])

        # Lint error because fx/fx.go should call ProvideComponentConstructor
        self.reset_component_src_in_tmpdir()

        filename = os.path.join(comps[3].path, 'fx/fx.go')
        replace_line(filename, '\t\tfxutil.ProvideComponentConstructor(', '\t\tfx.Provide(')

        comps, _ = components.get_components_and_bundles()
        errs = components.validate_components(comps)
        self.assertEqual(1, len(errs))
        self.assertIn('should call', errs[0])

        # Lint error because fx/ source file must be fx.go
        self.reset_component_src_in_tmpdir()

        badfilename = os.path.join(comps[3].path, 'fx/effeks.go')
        shutil.move(filename, badfilename)

        comps, _ = components.get_components_and_bundles()
        errs = components.validate_components(comps)
        self.assertEqual(1, len(errs))
        self.assertIn("should be named 'fx.go'", errs[0])

    def test_validate_component_implementation(self):
        comps, _ = components.get_components_and_bundles()
        errs = components.validate_components(comps)
        self.assertEqual(0, len(errs))

        # Lint error because implementation uses wrong package name
        filename = os.path.join(comps[3].path, 'impl/newstyle.go')
        replace_line(filename, 'package newstyleimpl', 'package impl')

        comps, _ = components.get_components_and_bundles()
        errs = components.validate_components(comps)
        self.assertEqual(1, len(errs))
        self.assertIn('wrong package name', errs[0])

        # Lint error because implementation imports uber fx
        self.reset_component_src_in_tmpdir()

        filename = os.path.join(comps[3].path, 'impl/newstyle.go')
        insert_after_line(filename, 'import (', '\t"go.uber.org/fx"')

        comps, _ = components.get_components_and_bundles()
        errs = components.validate_components(comps)
        self.assertEqual(1, len(errs))
        self.assertIn('should not import', errs[0])

        # Lint error because implementation imports fxutil
        self.reset_component_src_in_tmpdir()

        filename = os.path.join(comps[3].path, 'impl/newstyle.go')
        insert_after_line(filename, 'import (', '\t"github.com/DataDog/datadog-agent/pkg/util/fxutil"')

        comps, _ = components.get_components_and_bundles()
        errs = components.validate_components(comps)
        self.assertEqual(1, len(errs))
        self.assertIn('should not import', errs[0])

        # Okay for implementation to use a suffix for its non-primary implementation
        self.reset_component_src_in_tmpdir()

        implfolder = os.path.join(comps[3].path, 'impl')
        newfolder = os.path.join(comps[3].path, 'impl-alt')
        shutil.move(implfolder, newfolder)

        comps, _ = components.get_components_and_bundles()
        errs = components.validate_components(comps)
        self.assertEqual(0, len(errs))

        # Lint error because implementation doesn't use the 'impl' or 'impl-<suffix>' folder
        oldfolder = os.path.join(comps[3].path, 'impl-alt')
        newfolder = os.path.join(comps[3].path, 'implements')
        shutil.move(oldfolder, newfolder)

        comps, _ = components.get_components_and_bundles()
        errs = components.validate_components(comps)
        self.assertEqual(1, len(errs))
        self.assertIn('missing the implementation folder', errs[0])


def remove_line(filename, target):
    lines = components.read_file_content(filename).split('\n')
    content = '\n'.join([line for line in lines if line != target])
    fout = open(filename, 'w')
    fout.write(content)
    fout.close()


def replace_line(filename, target, replace):
    lines = components.read_file_content(filename).split('\n')
    content = '\n'.join([line if line != target else replace for line in lines])
    fout = open(filename, 'w')
    fout.write(content)
    fout.close()


def insert_after_line(filename, target, replace):
    actual_replace = f"{target}\n{replace}"
    replace_line(filename, target, actual_replace)


def append_line(filename, text):
    fout = open(filename, 'a')
    fout.write('\n' + text + '\n')
    fout.close()
