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
        mod = GoModule(path='pkg/my/module', test_targets=['.'], lint_targets=['.'], should_test_condition='always')
        self.assertTrue(mod.should_test())

    def test_never(self):
        mod = GoModule(path='pkg/my/module', test_targets=['.'], lint_targets=['.'], should_test_condition='never')
        self.assertFalse(mod.should_test())

    def test_error(self):
        mod = GoModule(path='pkg/my/module', test_targets=['.'], lint_targets=['.'], should_test_condition='???')
        self.assertRaises(KeyError, mod.should_test)


class TestGoModuleSerialization(unittest.TestCase):
    def test_to_dict(self):
        module = GoModule(
            path='pkg/my/module',
            test_targets=['.'],
            lint_targets=['.'],
            should_test_condition='always',
            should_tag=True,
            importable=True,
            independent=True,
            used_by_otel=True,
        )
        d = module.to_dict(remove_defaults=False)
        self.assertEqual(d['path'], module.path)
        self.assertEqual(d['should_test_condition'], module.should_test_condition)
        self.assertEqual(d['used_by_otel'], module.used_by_otel)

    def test_to_dict_defaults(self):
        module = GoModule(
            path='pkg/my/module',
            should_test_condition='never',
        )
        d = module.to_dict()

        # Default values are not present
        self.assertDictEqual(d, {'path': module.path, 'should_test_condition': module.should_test_condition})

    def test_from_dict(self):
        d = {
            'path': 'pkg/my/module',
            'test_targets': ['.'],
            'lint_targets': ['.'],
            'should_test_condition': 'always',
            'should_tag': True,
            'importable': True,
            'independent': True,
            'used_by_otel': True,
        }
        module = GoModule.from_dict(d['path'], d)

        self.assertEqual(d['path'], module.path)
        self.assertEqual(d['should_test_condition'], module.should_test_condition)
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
            'test_targets': ['.'],
            'lint_targets': ['.'],
            'should_test_condition': 'always',
            'should_tag': True,
            'importable': True,
            'independent': True,
            'used_by_otel': True,
            'legacy_go_mod_version': None,
        }
        module = GoModule.from_dict(d['path'], d)
        d2 = module.to_dict(remove_defaults=False)
        self.assertDictEqual(d, d2)

        module2 = GoModule.from_dict(d2['path'], d2)

        self.assertEqual(module2.path, module.path)
        self.assertEqual(module2.should_test_condition, module.should_test_condition)
        self.assertEqual(module2.used_by_otel, module.used_by_otel)

    def test_get_default_modules(self):
        # Ensure modules are loaded
        modules = get_default_modules()

        self.assertGreater(len(modules), 0)

    def test_ignored_modules(self):
        # Ensure ignored modules are not loaded
        config = Configuration.from_file()

        # Ensure there are ignored modules
        self.assertGreater(len(config.ignored_modules), 0)
        self.assertGreater(len(config.modules), 0)
        self.assertTrue(config.ignored_modules.isdisjoint(config.modules))

    def test_get_default_modules_base(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir = Path(tmpdir)
            paths = ['pkg/my/module', 'utils/a', 'utils/b']
            conditions = ['always', 'never', 'always']
            used_by_otel = [True, False, False]

            # Create modules
            modules = {
                path: GoModule(
                    path=path,
                    test_targets=['.'],
                    lint_targets=['.'],
                    should_test_condition=condition,
                    used_by_otel=used_by_otel,
                )
                for (path, condition, used_by_otel) in zip(paths, conditions, used_by_otel, strict=True)
            }
            Configuration(base_dir=tmpdir, modules=modules, ignored_modules=set()).to_file()

            self.assertTrue((Path(tmpdir) / Configuration.FILE_NAME).is_file())

            # Load modules
            modules_loaded = get_default_modules(base_dir=Path(tmpdir))

            self.assertDictEqual(modules, modules_loaded)


class TestGoModuleConfiguration(unittest.TestCase):
    def test_from(self):
        config = {
            'modules': {
                '.': {
                    'test_targets': ['pkg/my/module'],
                    'lint_targets': ['pkg/my/module'],
                    'should_test_condition': 'always',
                },
            }
        }
        modules = Configuration.from_dict(config).modules

        self.assertEqual(len(modules), 1)
        self.assertEqual(modules['.'].should_test_condition, 'always')

    def test_from_default(self):
        config = {
            'modules': {
                '.': {
                    'test_targets': ['pkg/my/module'],
                    'lint_targets': ['pkg/my/module'],
                    'should_test_condition': 'always',
                },
                'default': 'default',
            }
        }
        modules = Configuration.from_dict(config).modules

        self.assertEqual(len(modules), 2)
        self.assertEqual(modules['default'].to_dict(), {'path': 'default'})
        self.assertEqual(modules['default'].should_test_condition, GoModule('').should_test_condition)

    def test_from_ignored(self):
        config = {
            'modules': {
                '.': {
                    'test_targets': ['pkg/my/module'],
                    'lint_targets': ['pkg/my/module'],
                    'should_test_condition': 'always',
                },
                'ignored': 'ignored',
            }
        }
        c = Configuration.from_dict(config)

        self.assertEqual(len(c.modules), 1)
        self.assertEqual(c.ignored_modules, {'ignored'})

    def test_to(self):
        c = Configuration(
            base_dir=Path.cwd(), modules={'mod': GoModule('mod', should_test_condition='never')}, ignored_modules=set()
        )
        config = c.to_dict()

        self.assertEqual(len(config['modules']), 1)
        self.assertDictEqual(config['modules']['mod'], {'should_test_condition': 'never'})

    def test_to_default(self):
        c = Configuration(
            base_dir=Path.cwd(),
            modules={'mod': GoModule('mod', should_test_condition='never'), 'default': GoModule('default')},
            ignored_modules=set(),
        )
        config = c.to_dict()

        self.assertEqual(len(config['modules']), 2)
        self.assertEqual(config['modules']['default'], 'default')

    def test_to_ignored(self):
        c = Configuration(
            base_dir=Path.cwd(),
            modules={'mod': GoModule('mod', should_test_condition='never')},
            ignored_modules={'ignored'},
        )
        config = c.to_dict()

        self.assertEqual(len(config['modules']), 2)
        self.assertEqual(config['modules']['ignored'], 'ignored')
