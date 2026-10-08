import os
import unittest

import yaml
from invoke.exceptions import Exit

from tasks.schema.merge_schema import resolve_schema
from tasks.schema.show_product import list_products, product_config, render_config, resolve_products

SCHEMA_DIR = os.path.join("pkg", "config", "schema", "yaml")

CORE = {
    "sku_definitions": {"sku_a": ["product_a"]},
    "product_dependencies": {
        "product_a": {"dependencies": ["product_b"]},
        "product_b": {"dependencies": []},
        "product_c": {"dependencies": [], "conflict": ["product_d"]},
        "product_d": {
            "dependencies": [],
            "profiles": {
                "product_d_fast": ["product_b"],
                "product_d_small": {"dependencies": [], "conflict": ["product_a"]},
            },
        },
    },
    "properties": {
        "logs_enabled": {
            "node_type": "setting",
            "type": "boolean",
            "default": False,
            "product_defaults": {"product_a": True, "product_c": False},
        },
        "apm_enabled": {
            "node_type": "setting",
            "type": "boolean",
            "default": True,
            # same as the regular default: not part of the generated configuration
            "product_defaults": {"product_b": True},
        },
        "logs_config": {
            "node_type": "section",
            "properties": {
                "container_collect_all": {
                    "node_type": "setting",
                    "type": "boolean",
                    "default": False,
                    "product_platform_defaults": {"product_b": {"container": True}},
                },
                "use_file": {
                    "node_type": "setting",
                    "type": "boolean",
                    "default": False,
                    "product_platform_defaults": {"product_b": {"kubernetes": True}},
                },
            },
        },
    },
}

SYSTEM_PROBE = {
    "properties": {
        "network_config": {
            "node_type": "section",
            "properties": {
                "enabled": {
                    "node_type": "setting",
                    "type": "boolean",
                    "default": False,
                    "product_platform_defaults": {"product_b": {"linux": True, "fargate": False}},
                }
            },
        }
    }
}


class TestResolveProducts(unittest.TestCase):
    def test_dependencies_are_resolved(self):
        self.assertEqual(resolve_products(["product_a"], CORE), ["product_a", "product_b"])

    def test_unknown_product(self):
        with self.assertRaisesRegex(Exit, "unknown product 'missing'"):
            resolve_products(["missing"], CORE)

    def test_sku_is_not_a_product(self):
        with self.assertRaisesRegex(Exit, "'sku_a' is a SKU"):
            resolve_products(["sku_a"], CORE)

    def test_no_product(self):
        with self.assertRaisesRegex(Exit, "at least one product"):
            resolve_products([], CORE)


class TestProductConfig(unittest.TestCase):
    def test_linux_host(self):
        core, system_probe = product_config(CORE, SYSTEM_PROBE, "linux", ["product_a", "product_b"])
        self.assertEqual(core, {"logs_enabled": True})
        self.assertEqual(system_probe, {"network_config": {"enabled": True}})

    def test_kubernetes_falls_back_to_container_then_linux(self):
        core, system_probe = product_config(CORE, SYSTEM_PROBE, "kubernetes", ["product_b"])
        self.assertEqual(core, {"logs_config": {"container_collect_all": True, "use_file": True}})
        self.assertEqual(system_probe, {"network_config": {"enabled": True}})

    def test_fargate(self):
        # fargate: false is the regular default, so it isn't part of the configuration
        core, system_probe = product_config(CORE, SYSTEM_PROBE, "fargate", ["product_b"])
        self.assertEqual(core, {"logs_config": {"container_collect_all": True}})
        self.assertEqual(system_probe, {})

    def test_conflict(self):
        with self.assertRaisesRegex(Exit, "logs_enabled: product_a=True, product_c=False"):
            product_config(CORE, SYSTEM_PROBE, "linux", ["product_a", "product_c"])

    def test_unknown_platform(self):
        with self.assertRaisesRegex(Exit, "unknown platform 'solaris'"):
            product_config(CORE, SYSTEM_PROBE, "solaris", ["product_a"])


class TestRenderConfig(unittest.TestCase):
    def test_both_files(self):
        output = render_config({"logs_enabled": True}, {"network_config": {"enabled": True}}, "linux")
        documents = output.split("---\n")
        self.assertEqual(len(documents), 2)
        self.assertTrue(documents[0].startswith("# datadog.yaml\n"))
        self.assertEqual(yaml.safe_load(documents[0]), {"logs_enabled": True})
        self.assertTrue(documents[1].startswith("# system-probe.yaml\n"))
        self.assertEqual(yaml.safe_load(documents[1]), {"network_config": {"enabled": True}})

    def test_empty_file_is_omitted(self):
        output = render_config({"logs_enabled": True}, {}, "linux")
        self.assertNotIn("system-probe.yaml", output)

    def test_nothing_to_configure(self):
        output = render_config({}, {}, "linux")
        self.assertIn("don't change the default configuration on 'linux'", output)


class TestRealSchema(unittest.TestCase):
    def test_log_management_collect_all_on_kubernetes(self):
        core = resolve_schema(os.path.join(SCHEMA_DIR, "core_schema.yaml"))
        system_probe = resolve_schema(os.path.join(SCHEMA_DIR, "system-probe_schema.yaml"))
        core_config, system_probe_config = product_config(
            core, system_probe, "kubernetes", ["log_management_collect_all"]
        )
        self.assertEqual(
            core_config,
            {"logs_enabled": True, "logs_config": {"container_collect_all": True, "k8s_container_use_file": True}},
        )
        self.assertEqual(system_probe_config, {})


class TestProfilesAndConflicts(unittest.TestCase):
    def test_profile_enables_its_product_and_dependencies(self):
        self.assertEqual(resolve_products(["product_d_fast"], CORE), ["product_b", "product_d", "product_d_fast"])

    def test_one_profile_per_product(self):
        with self.assertRaisesRegex(Exit, "only one profile of 'product_d'"):
            resolve_products(["product_d_fast", "product_d_small"], CORE)

    def test_explicit_conflict(self):
        with self.assertRaisesRegex(Exit, "'product_c' and 'product_d' can't be enabled together"):
            resolve_products(["product_c", "product_d"], CORE)

    def test_explicit_conflict_through_dependencies(self):
        # product_d_small conflicts with product_a; product_a is enabled by itself here
        with self.assertRaisesRegex(Exit, "'product_a' and 'product_d_small' can't be enabled together"):
            resolve_products(["product_a", "product_d_small"], CORE)

    def test_conflict_through_profile_owner(self):
        # product_d_fast enables product_d, which conflicts with product_c
        with self.assertRaisesRegex(Exit, "'product_c' and 'product_d' can't be enabled together"):
            resolve_products(["product_c", "product_d_fast"], CORE)


class TestListProducts(unittest.TestCase):
    def test_products_profiles_dependencies_and_conflicts(self):
        lines = list_products(CORE).splitlines()
        self.assertEqual(lines[0].split(), ["PRODUCT", "/", "PROFILE", "DEPENDS", "ON", "CONFLICTS", "WITH"])
        rows = [line.split() for line in lines[1:5]]
        self.assertEqual(rows[0], ["product_a", "product_b", "product_d_small"])
        self.assertEqual(rows[1], ["product_b", "-", "-"])
        self.assertEqual(rows[2], ["product_c", "-", "product_d"])
        self.assertEqual(rows[3], ["product_d", "-", "product_c"])
        # profiles are listed under their product, indented
        self.assertTrue(lines[5].startswith("  product_d_fast"))
        self.assertEqual(lines[5].split(), ["product_d_fast", "product_b", "-"])
        self.assertEqual(lines[6].split(), ["product_d_small", "-", "product_a"])

    def test_skus_are_listed(self):
        output = list_products(CORE)
        self.assertIn("SKU", output)
        self.assertRegex(output, r"sku_a\s+product_a")

    def test_no_sku_section_without_skus(self):
        self.assertNotIn("SKU", list_products({"product_dependencies": {"product_a": {"dependencies": []}}}))

    def test_real_schema(self):
        output = list_products(resolve_schema(os.path.join(SCHEMA_DIR, "core_schema.yaml")))
        self.assertRegex(output, r"cloud_siem\s+log_management")
        self.assertRegex(output, r"\n  log_management_high_throughput\s+log_management_high_concurrency")
        self.assertRegex(output, r"end_user_device_monitoring\s+infrastructure_mode_end_user_device")
        self.assertRegex(
            output,
            r"network_monitoring\s+cloud_network_monitoring, netflow_monitoring, network_device_monitoring, network_path",
        )


class TestClusterAgentPlatform(unittest.TestCase):
    def test_cluster_agent_falls_back_to_kubernetes(self):
        core, _ = product_config(CORE, SYSTEM_PROBE, "cluster_agent", ["product_b"])
        self.assertEqual(core, {"logs_config": {"container_collect_all": True, "use_file": True}})


class TestIncludeDefaults(unittest.TestCase):
    def test_values_equal_to_the_default_are_included(self):
        core, system_probe = product_config(CORE, SYSTEM_PROBE, "linux", ["product_b"], include_defaults=True)
        # product_b sets apm_enabled to its regular default (true)
        self.assertEqual(core, {"apm_enabled": True})
        self.assertEqual(system_probe, {"network_config": {"enabled": True}})

    def test_values_equal_to_the_default_are_omitted_by_default(self):
        core, _ = product_config(CORE, SYSTEM_PROBE, "linux", ["product_b"])
        self.assertEqual(core, {})

    def test_platform_value_equal_to_the_default(self):
        # on fargate product_b sets network_config.enabled to false, the regular default
        _, system_probe = product_config(CORE, SYSTEM_PROBE, "fargate", ["product_b"], include_defaults=True)
        self.assertEqual(system_probe, {"network_config": {"enabled": False}})

    def test_real_schema_apm(self):
        core = resolve_schema(os.path.join(SCHEMA_DIR, "core_schema.yaml"))
        system_probe = resolve_schema(os.path.join(SCHEMA_DIR, "system-probe_schema.yaml"))
        core_config, _ = product_config(core, system_probe, "linux", ["apm"], include_defaults=True)
        self.assertEqual(core_config, {"apm_config": {"enabled": True}})
