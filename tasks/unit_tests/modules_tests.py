from __future__ import annotations

import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path
from typing import Any

from tasks.libs.common.gomodules import (
    AGENT_MODULE_PATH_PREFIX,
    Configuration,
    GoModule,
    get_default_modules,
    list_default_modules,
)

"""
Here is an abstract of the go.mod file format:

{
    "Module": {"Path": "github.com/DataDog/datadog-agent"},
    "Go": "1.22",
    "Require": [
        {"Path": "github.com/DataDog/datadog-agent/pkg/config/logs", "Version": "v0.51.0-rc.2"},
        {"Path": "k8s.io/kms", "Version": "v0.27.6", "Indirect": true},
    ],
    "Exclude": [
        {"Path": "github.com/knadh/koanf/maps", "Version": "v0.1.1"},
        {"Path": "github.com/knadh/koanf/providers/confmap", "Version": "v0.1.0"},
    ],
    "Replace": [
        {
            "Old": {"Path": "github.com/cihub/seelog"},
            "New": {"Path": "github.com/cihub/seelog", "Version": "v0.0.0-20151216151435-d2c6e5aa9fbf"},
        },
        {
            "Old": {"Path": "github.com/DataDog/datadog-agent/pkg/util/defaultpaths"},
            "New": {"Path": "./pkg/util/defaultpaths/"},
        },
    ],
    "Retract": [{"Low": "v0.9.0", "High": "v0.9.0"}, {"Low": "v0.8.0", "High": "v0.8.0"}],
}
"""


class TestModules(unittest.TestCase):
    def load_go_mod(self, module_path: str) -> Any:
        """Loads the go.mod file as a JSON object"""
        go_mod_path = os.path.join(module_path, "go.mod")
        res = subprocess.run(["go", "mod", "edit", "-json", go_mod_path], capture_output=True)
        self.assertEqual(res.returncode, 0)

        return json.loads(res.stdout)

    def get_agent_required(self, module: dict) -> set[str]:
        """Returns the set of required datadog-agent modules"""
        if "Require" not in module:
            return set()

        required = module["Require"]
        if required is None:
            return set()

        results = set()
        self.assertIsInstance(required, list)
        for req in required:
            self.assertIsInstance(req, dict)
            self.assertIn("Path", req)
            path = req["Path"]

            self.assertIsInstance(path, str)
            if path.startswith(AGENT_MODULE_PATH_PREFIX):
                results.add(path)

        return results

    def get_agent_replaced(self, module: dict) -> set[str]:
        """Returns the set of replaced datadog-agent modules"""
        if "Replace" not in module:
            return set()

        replaced = module["Replace"]
        if replaced is None:
            return set()

        results = set()
        self.assertIsInstance(replaced, list)
        for req in replaced:
            self.assertIsInstance(req, dict)
            self.assertIn("Old", req)
            old = req["Old"]

            self.assertIsInstance(old, dict)
            self.assertIn("Path", old)
            oldpath = old["Path"]
            if oldpath.startswith(AGENT_MODULE_PATH_PREFIX):
                results.add(oldpath)

        return results

    def test_modules_replace_agent(self):
        """Ensure that all required datadog-agent modules are replaced"""
        for module_path in get_default_modules().keys():
            with self.subTest(module_path=module_path):
                module = self.load_go_mod(module_path)
                self.assertIsInstance(module, dict)
                required = self.get_agent_required(module)
                replaced = self.get_agent_replaced(module)
                required_not_replaced = required - replaced
                self.assertEqual(required_not_replaced, set(), f"in module {module_path}")


class TestGoModuleCondition(unittest.TestCase):
    def test_always(self):
        mod = GoModule(path='pkg/my/module', targets=['.'], lint_targets=['.'], condition='always')
        self.assertTrue(mod.verify_condition())

    def test_never(self):
        mod = GoModule(path='pkg/my/module', targets=['.'], lint_targets=['.'], condition='never')
        self.assertFalse(mod.verify_condition())

    def test_error(self):
        mod = GoModule(path='pkg/my/module', targets=['.'], lint_targets=['.'], condition='???')
        self.assertRaises(KeyError, mod.verify_condition)


class TestGoModuleSerialization(unittest.TestCase):
    def test_to_dict(self):
        module = GoModule(
            path='pkg/my/module',
            targets=['.'],
            lint_targets=['.'],
            condition='always',
            should_tag=True,
            importable=True,
            independent=True,
            used_by_otel=True,
        )
        d = module.to_dict(remove_defaults=False)
        self.assertEqual(d['path'], module.path)
        self.assertEqual(d['condition'], module.condition)
        self.assertEqual(d['used_by_otel'], module.used_by_otel)

    def test_to_dict_defaults(self):
        module = GoModule(
            path='pkg/my/module',
            condition='never',
        )
        d = module.to_dict()

        # Default values are not present
        self.assertDictEqual(d, {'path': module.path, 'condition': module.condition})

    def test_from_dict(self):
        d = {
            'path': 'pkg/my/module',
            'targets': ['.'],
            'lint_targets': ['.'],
            'condition': 'always',
            'should_tag': True,
            'importable': True,
            'independent': True,
            'used_by_otel': True,
        }
        module = GoModule.from_dict(d['path'], d)

        self.assertEqual(d['path'], module.path)
        self.assertEqual(d['condition'], module.condition)
        self.assertEqual(d['used_by_otel'], module.used_by_otel)

    def test_from_dict_defaults(self):
        mod = GoModule.from_dict('pkg/my/module', {})
        mod2 = GoModule.from_dict('pkg/my/module', {'should_tag': True})
        mod3 = GoModule.from_dict('pkg/my/module', {'should_tag': False})

        self.assertEqual(mod.should_tag, True)
        self.assertEqual(mod2.should_tag, True)
        self.assertEqual(mod3.should_tag, False)

    def test_from_to(self):
        d = {
            'path': 'pkg/my/module',
            'targets': ['.'],
            'lint_targets': ['.'],
            'condition': 'always',
            'should_tag': True,
            'importable': True,
            'independent': True,
            'used_by_otel': True,
        }
        module = GoModule.from_dict(d['path'], d)
        d2 = module.to_dict(remove_defaults=False)
        self.assertDictEqual(d, d2)

        module2 = GoModule.from_dict(d2['path'], d2)

        self.assertEqual(module2.path, module.path)
        self.assertEqual(module2.condition, module.condition)
        self.assertEqual(module2.used_by_otel, module.used_by_otel)

    def test_from_to_file(self):
        path = 'pkg/my/module'
        module = GoModule(
            path=path,
            targets=['.'],
            lint_targets=['.'],
            condition='always',
            should_tag=True,
            importable=True,
            independent=True,
            used_by_otel=True,
        )

        with tempfile.TemporaryDirectory() as tmpdir:
            (Path(tmpdir) / path).mkdir(parents=True, exist_ok=True)

            module.to_file(base_dir=Path(tmpdir))
            module2 = GoModule.from_file(path, base_dir=Path(tmpdir))

        # Remove temp file prefix
        self.assertEqual(module2.path, module.path)
        self.assertEqual(module2.condition, module.condition)
        self.assertEqual(module2.used_by_otel, module.used_by_otel)

    def test_get_default_modules(self):
        # Ensure modules are loaded
        modules = get_default_modules()

        self.assertGreater(len(modules), 0)

    def test_ignored_modules(self):
        # Ensure ignored modules are not loaded
        _, ignored_modules = list_default_modules()
        modules = set(get_default_modules())

        # Ensure there are ignored modules
        self.assertGreater(len(ignored_modules), 0)
        self.assertGreater(len(modules), 0)
        self.assertTrue(ignored_modules.isdisjoint(modules))

    def test_get_default_modules_base(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)
            paths = ['pkg/my/module', 'utils/a', 'utils/b']
            conditions = ['always', 'never', 'always']
            used_by_otel = [True, False, False]

            # Create modules
            modules = {
                path: GoModule(
                    path=path, targets=['.'], lint_targets=['.'], condition=condition, used_by_otel=used_by_otel
                )
                for (path, condition, used_by_otel) in zip(paths, conditions, used_by_otel, strict=True)
            }
            for module in modules.values():
                (tmpdir / module.path).mkdir(parents=True, exist_ok=True)
                module.to_file(base_dir=tmpdir)
                (tmpdir / module.path / 'go.mod').touch()

            # Load modules
            modules_loaded = get_default_modules(base_dir=Path(tmpdir))

            # Reset base_dir which differs from both collections
            for module in modules_loaded.values():
                module.base_dir = Path.cwd()

            self.assertDictEqual(modules, modules_loaded)

    def test_module_default(self):
        """
        If no `module.yml` file is present, a default module should be created.
        """

        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)
            path = 'pkg/my/module'
            (tmpdir / path).mkdir(parents=True, exist_ok=True)

            module = GoModule.from_file(path, base_dir=tmpdir)
            default_module = GoModule(path=path)

            self.assertEqual(module.path, path)
            self.assertEqual(module.condition, default_module.condition)
            self.assertEqual(module.should_tag, default_module.should_tag)
            self.assertEqual(module.targets, default_module.targets)

    def test_module_ignored(self):
        """
        If no `module.yml` file is present, a default module should be created.
        """

        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)
            path = 'pkg/my/module'
            (tmpdir / path).mkdir(parents=True, exist_ok=True)

            with open(tmpdir / path / 'module.yml', 'w') as f:
                f.write('ignored: true\n')

            module = GoModule.from_file(path, base_dir=tmpdir)

            self.assertIsNone(module)


class TestGoModulePath(unittest.TestCase):
    def assert_path_equal(self, path1: Path | str, path2: Path | str):
        path1 = path1 if isinstance(path1, Path) else Path(path1)
        path2 = path2 if isinstance(path2, Path) else Path(path2)

        self.assertEqual(path1.absolute().as_posix(), path2.absolute().as_posix())

    def test_parse_path_default(self):
        module_path, base_dir, dir_path, full_path = GoModule.parse_path(dir_path='pkg/my/module')
        self.assert_path_equal(module_path, 'pkg/my/module')
        self.assert_path_equal(base_dir, '.')
        self.assert_path_equal(dir_path, Path('pkg/my/module'))
        self.assert_path_equal(full_path, Path('./pkg/my/module'))

    def test_parse_path_base(self):
        module_path, base_dir, dir_path, full_path = GoModule.parse_path(dir_path='pkg/my/module', base_dir='../agent6')
        self.assert_path_equal(module_path, 'pkg/my/module')
        self.assert_path_equal(base_dir, '../agent6')
        self.assert_path_equal(dir_path, Path('pkg/my/module'))
        self.assert_path_equal(full_path, Path('../agent6/pkg/my/module'))

    def test_full_path(self):
        module = GoModule('pkg/my/module')

        self.assertEqual(module.full_path(), str(Path('pkg/my/module').resolve()))

    def test_full_path_base(self):
        module = GoModule('pkg/my/module', base_dir='/tmp')

        self.assertEqual(module.full_path(), str(Path('/tmp/pkg/my/module').resolve()))

    def test_load_modules_path(self):
        with tempfile.TemporaryDirectory() as temp:
            temp = Path(temp)
            (temp / 'pkg/my/module').mkdir(parents=True, exist_ok=True)
            (temp / 'pkg/my/module' / 'go.mod').touch()
            with open(temp / 'pkg/my/module' / 'module.yml', 'w') as f:
                print('independent: true', file=f)

            modules = get_default_modules(base_dir=temp)
            self.assertEqual(len(modules), 1)
            mod = next(iter(modules.values()))
            self.assertEqual(mod.path, 'pkg/my/module')
            self.assertEqual(mod.full_path(), str(Path(temp / 'pkg/my/module').resolve()))


class TestGoModuleConfiguration(unittest.TestCase):
    def test_from(self):
        config = {
            'modules': {
                '.': {'targets': ['pkg/my/module'], 'lint_targets': ['pkg/my/module'], 'condition': 'always'},
            }
        }
        modules = Configuration.from_dict(config).modules

        self.assertEqual(len(modules), 1)
        self.assertEqual(modules['.'].condition, 'always')

    def test_from_default(self):
        config = {
            'modules': {
                '.': {'targets': ['pkg/my/module'], 'lint_targets': ['pkg/my/module'], 'condition': 'always'},
                'default': 'default',
            }
        }
        modules = Configuration.from_dict(config).modules

        self.assertEqual(len(modules), 2)
        self.assertEqual(modules['default'].to_dict(), {'path': 'default'})
        self.assertEqual(modules['default'].condition, GoModule('').condition)

    def test_from_ignored(self):
        config = {
            'modules': {
                '.': {'targets': ['pkg/my/module'], 'lint_targets': ['pkg/my/module'], 'condition': 'always'},
                'ignored': 'ignored',
            }
        }
        c = Configuration.from_dict(config)

        self.assertEqual(len(c.modules), 1)
        self.assertEqual(c.ignored_modules, {'ignored'})

    def test_to(self):
        c = Configuration(
            base_dir=Path.cwd(), modules={'mod': GoModule('mod', condition='never')}, ignored_modules=set()
        )
        config = c.to_dict()

        self.assertEqual(len(config['modules']), 1)
        self.assertDictEqual(config['modules']['mod'], {'condition': 'never'})

    def test_to_default(self):
        c = Configuration(
            base_dir=Path.cwd(),
            modules={'mod': GoModule('mod', condition='never'), 'default': GoModule('default')},
            ignored_modules=set(),
        )
        config = c.to_dict()

        self.assertEqual(len(config['modules']), 2)
        self.assertEqual(config['modules']['default'], 'default')

    def test_to_ignored(self):
        c = Configuration(
            base_dir=Path.cwd(), modules={'mod': GoModule('mod', condition='never')}, ignored_modules={'ignored'}
        )
        config = c.to_dict()

        self.assertEqual(len(config['modules']), 2)
        self.assertEqual(config['modules']['ignored'], 'ignored')
