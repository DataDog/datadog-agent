import unittest


class TestJsonSchemaOutput(unittest.TestCase):
    def test_keeps_only_standard_keywords(self):
        from tasks.schema.produce_byproduct import json_schema

        doc = {
            "type": "object",
            "node_type": "section",
            "visibility": "public",
            "properties": {
                "enabled": {
                    "type": "boolean",
                    "default": False,
                    "env_vars": ["DD_ENABLED"],
                },
            },
        }
        self.assertEqual(
            json_schema(doc),
            {
                "type": "object",
                "properties": {
                    "enabled": {
                        "type": "boolean",
                        "default": False,
                    },
                },
            },
        )

    def test_object_default_is_preserved_verbatim(self):
        """Regression: instance data under ``default`` must not be walked as a
        schema subtree, or its non-keyword keys are dropped to ``{}``."""
        from tasks.schema.produce_byproduct import json_schema

        doc = {
            "type": "object",
            "properties": {
                "kubernetes_node_annotations_as_tags": {
                    "type": "object",
                    "default": {"cluster.k8s.io/machine": "kube_machine"},
                    "additionalProperties": {"type": "string"},
                },
            },
        }
        result = json_schema(doc)
        self.assertEqual(
            result["properties"]["kubernetes_node_annotations_as_tags"]["default"],
            {"cluster.k8s.io/machine": "kube_machine"},
        )

    def test_enum_const_examples_instance_data_preserved(self):
        from tasks.schema.produce_byproduct import json_schema

        doc = {
            "enum": [{"description": "not a keyword here"}, "plain"],
            "const": {"properties": "instance value"},
            "examples": [{"cluster.k8s.io/machine": "machine"}],
        }
        self.assertEqual(json_schema(doc), doc)


if __name__ == "__main__":
    unittest.main()


class TestProductEnablementKeywords(unittest.TestCase):
    def _doc(self):
        return {
            "sku_definitions": {"sku_a": ["product_a"]},
            "product_dependencies": {"product_a": ["product_b"], "product_b": []},
            "properties": {
                "logs_enabled": {
                    "type": "boolean",
                    "default": False,
                    "description": "dropped by embedded",
                    "product_defaults": {"product_a": True},
                },
                "logs_config": {
                    "type": "object",
                    "properties": {
                        "tags": {
                            "type": "object",
                            "default": {},
                            # Values are instance data: keys like 'title' must never be stripped.
                            "product_platform_defaults": {
                                "product_a": {"container": {"title": "x", "description": "y"}, "other": {}},
                            },
                        },
                    },
                },
            },
        }

    def test_embedded_keeps_product_keywords_verbatim(self):
        from tasks.schema.produce_byproduct import embedded

        out = embedded(self._doc())
        self.assertEqual(out["sku_definitions"], {"sku_a": ["product_a"]})
        self.assertEqual(out["product_dependencies"], {"product_a": ["product_b"], "product_b": []})
        self.assertEqual(out["properties"]["logs_enabled"]["product_defaults"], {"product_a": True})
        self.assertNotIn("description", out["properties"]["logs_enabled"])
        self.assertEqual(
            out["properties"]["logs_config"]["properties"]["tags"]["product_platform_defaults"],
            {"product_a": {"container": {"title": "x", "description": "y"}, "other": {}}},
        )

    def test_json_schema_drops_product_keywords(self):
        from tasks.schema.produce_byproduct import json_schema

        out = json_schema(self._doc())
        self.assertNotIn("sku_definitions", out)
        self.assertNotIn("product_dependencies", out)
        self.assertNotIn("product_defaults", out["properties"]["logs_enabled"])
        self.assertNotIn("product_platform_defaults", out["properties"]["logs_config"]["properties"]["tags"])
