import os
import unittest

import tasks.schema.lint as lint

TESTDATA = os.path.join(os.path.dirname(__file__), "testdata", "schema_lint")


def fixture(name):
    return os.path.join(TESTDATA, name)


def errors_for(check_fn, filename, *args):
    """Helper: run a check function against a fixture file and return its errors."""
    import yaml

    with open(fixture(filename)) as f:
        schema = yaml.safe_load(f)
    return check_fn(fixture(filename), schema, *args)


class TestCheckYamlValid(unittest.TestCase):
    def test_valid_file_produces_no_errors(self):
        errors = lint.check_yaml_valid(fixture("valid.yaml"))
        self.assertEqual(errors, [])

    def test_invalid_yaml_produces_error(self):
        import tempfile

        with tempfile.NamedTemporaryFile(suffix=".yaml", mode="w", delete=False) as f:
            f.write("key: [unclosed bracket\n")
            path = f.name
        try:
            errors = lint.check_yaml_valid(path)
            self.assertTrue(len(errors) > 0, "Expected at least one error for invalid YAML")
            self.assertTrue(any("YAML" in e or "yaml" in e or "parse" in e.lower() for e in errors))
        finally:
            os.unlink(path)


class TestCheckJsonSchemaStructure(unittest.TestCase):
    def test_valid_schema_produces_no_errors(self):
        errors = errors_for(lint.check_json_schema_structure, "valid.yaml")
        self.assertEqual(errors, [])

    def test_invalid_type_value(self):
        errors = errors_for(lint.check_json_schema_structure, "bad_json_schema_structure.yaml")
        paths = [e for e in errors if "bad_type_value" in e]
        self.assertTrue(len(paths) > 0, f"Expected error for bad_type_value, got: {errors}")

    def test_array_without_items(self):
        errors = errors_for(lint.check_json_schema_structure, "bad_json_schema_structure.yaml")
        paths = [e for e in errors if "array_no_items" in e]
        self.assertTrue(len(paths) > 0, f"Expected error for array_no_items, got: {errors}")

    def test_array_without_items_excepted_passes(self):
        errors = errors_for(lint.check_json_schema_structure, "bad_json_schema_structure.yaml", {"array_no_items"})
        self.assertFalse(any("array_no_items" in e for e in errors))

    def test_valid_array_with_items_passes(self):
        errors = errors_for(lint.check_json_schema_structure, "valid.yaml")
        self.assertFalse(any("array_setting" in e for e in errors))


class TestCheckPublicDescriptions(unittest.TestCase):
    def test_valid_schema_produces_no_errors(self):
        errors = errors_for(lint.check_public_descriptions, "valid.yaml")
        self.assertEqual(errors, [])

    def test_public_setting_without_description(self):
        errors = errors_for(lint.check_public_descriptions, "missing_description.yaml")
        self.assertTrue(any("public_no_desc" in e for e in errors), f"Expected error for public_no_desc, got: {errors}")

    def test_public_setting_with_empty_description(self):
        errors = errors_for(lint.check_public_descriptions, "missing_description.yaml")
        self.assertTrue(
            any("public_empty_desc" in e for e in errors), f"Expected error for public_empty_desc, got: {errors}"
        )

    def test_private_setting_without_description_passes(self):
        errors = errors_for(lint.check_public_descriptions, "missing_description.yaml")
        self.assertFalse(any("private_no_desc" in e for e in errors))

    def test_public_section_without_description(self):
        errors = errors_for(lint.check_public_descriptions, "missing_description.yaml")
        self.assertTrue(
            any("public_section_no_desc" in e for e in errors),
            f"Expected error for public_section_no_desc, got: {errors}",
        )


class TestCheckPublicParentSections(unittest.TestCase):
    def test_valid_schema_produces_no_errors(self):
        errors = errors_for(lint.check_public_parent_sections, "valid.yaml")
        self.assertEqual(errors, [])

    def test_public_child_under_private_section(self):
        errors = errors_for(lint.check_public_parent_sections, "parent_not_public.yaml")
        self.assertTrue(
            any("private_section" in e for e in errors),
            f"Expected error for private_section.public_child, got: {errors}",
        )

    def test_public_child_under_section_without_visibility(self):
        errors = errors_for(lint.check_public_parent_sections, "parent_not_public.yaml")
        self.assertTrue(
            any("no_visibility_section" in e for e in errors),
            f"Expected error for no_visibility_section, got: {errors}",
        )

    def test_public_child_under_public_section_without_description(self):
        errors = errors_for(lint.check_public_parent_sections, "parent_not_public.yaml")
        self.assertTrue(
            any("public_section_no_desc" in e for e in errors),
            f"Expected error for public_section_no_desc (missing description), got: {errors}",
        )


class TestCheckNodeTypesPresent(unittest.TestCase):
    def test_valid_schema_produces_no_errors(self):
        errors = errors_for(lint.check_node_types_present, "valid.yaml")
        self.assertEqual(errors, [])

    def test_node_without_node_type(self):
        errors = errors_for(lint.check_node_types_present, "missing_node_type.yaml")
        self.assertTrue(any("no_node_type" in e for e in errors), f"Expected error for no_node_type, got: {errors}")

    def test_nested_node_without_node_type(self):
        errors = errors_for(lint.check_node_types_present, "missing_node_type.yaml")
        self.assertTrue(
            any("also_missing" in e for e in errors), f"Expected error for valid_section.also_missing, got: {errors}"
        )

    def test_node_with_invalid_node_type(self):
        errors = errors_for(lint.check_node_types_present, "missing_node_type.yaml")
        self.assertTrue(any("bad_node_type" in e for e in errors), f"Expected error for bad_node_type, got: {errors}")

    def test_good_node_type_passes(self):
        errors = errors_for(lint.check_node_types_present, "missing_node_type.yaml")
        self.assertFalse(any(e for e in errors if "good_setting" in e and "node_type" in e.lower()))


class TestCheckSettingsHaveDefault(unittest.TestCase):
    def test_valid_schema_produces_no_errors(self):
        errors = errors_for(lint.check_settings_have_default, "valid.yaml")
        self.assertEqual(errors, [])

    def test_setting_without_default(self):
        errors = errors_for(lint.check_settings_have_default, "missing_default.yaml")
        self.assertTrue(any("no_default" in e for e in errors), f"Expected error for no_default, got: {errors}")

    def test_setting_with_platform_default_passes(self):
        errors = errors_for(lint.check_settings_have_default, "missing_default.yaml")
        self.assertFalse(any("with_platform_default" in e for e in errors))


class TestCheckSettingsHaveType(unittest.TestCase):
    def test_valid_schema_produces_no_errors(self):
        errors = errors_for(lint.check_settings_have_type, "valid.yaml")
        self.assertEqual(errors, [])

    def test_setting_without_type(self):
        errors = errors_for(lint.check_settings_have_type, "missing_type.yaml")
        self.assertTrue(any("no_type" in e for e in errors), f"Expected error for no_type, got: {errors}")


class TestCheckPlatformDefaultKeys(unittest.TestCase):
    def test_valid_schema_produces_no_errors(self):
        errors = errors_for(lint.check_platform_default_keys, "valid.yaml")
        self.assertEqual(errors, [])

    def test_missing_required_platform_key(self):
        errors = errors_for(lint.check_platform_default_keys, "bad_platform_default.yaml")
        self.assertTrue(
            any("missing_windows" in e for e in errors), f"Expected error for missing_windows, got: {errors}"
        )

    def test_missing_aix_platform_key(self):
        errors = errors_for(lint.check_platform_default_keys, "bad_platform_default.yaml")
        self.assertTrue(any("missing_aix" in e for e in errors), f"Expected error for missing_aix, got: {errors}")

    def test_unknown_platform_key(self):
        errors = errors_for(lint.check_platform_default_keys, "bad_platform_default.yaml")
        self.assertTrue(any("unknown_key" in e for e in errors), f"Expected error for unknown_key, got: {errors}")

    def test_valid_with_other_passes(self):
        errors = errors_for(lint.check_platform_default_keys, "bad_platform_default.yaml")
        self.assertFalse(any("valid_with_other" in e for e in errors))

    def test_valid_all_platforms_passes(self):
        errors = errors_for(lint.check_platform_default_keys, "bad_platform_default.yaml")
        self.assertFalse(any("valid_all_platforms" in e for e in errors))


class TestCheckSectionsHaveChildren(unittest.TestCase):
    def test_valid_schema_produces_no_errors(self):
        errors = errors_for(lint.check_sections_have_children, "valid.yaml")
        self.assertEqual(errors, [])

    def test_section_with_empty_properties_is_error(self):
        errors = errors_for(lint.check_sections_have_children, "empty_section.yaml")
        self.assertTrue(
            any("empty_props_section" in e for e in errors),
            f"Expected error for empty_props_section, got: {errors}",
        )

    def test_section_with_no_properties_key_is_error(self):
        errors = errors_for(lint.check_sections_have_children, "empty_section.yaml")
        self.assertTrue(
            any("no_props_section" in e for e in errors),
            f"Expected error for no_props_section, got: {errors}",
        )

    def test_section_with_children_passes(self):
        errors = errors_for(lint.check_sections_have_children, "empty_section.yaml")
        self.assertFalse(any("valid_section" in e for e in errors))


class TestCheckPublicSectionHasPublicChild(unittest.TestCase):
    def test_valid_schema_produces_no_errors(self):
        errors = errors_for(lint.check_public_section_has_public_child, "valid.yaml")
        self.assertEqual(errors, [])

    def test_public_section_with_only_private_children_is_error(self):
        errors = errors_for(lint.check_public_section_has_public_child, "public_section_no_public_child.yaml")
        self.assertTrue(
            any("all_private_children" in e for e in errors),
            f"Expected error for all_private_children, got: {errors}",
        )

    def test_public_section_with_public_child_passes(self):
        errors = errors_for(lint.check_public_section_has_public_child, "public_section_no_public_child.yaml")
        self.assertFalse(any("has_public_child" in e for e in errors))

    def test_private_section_with_no_public_children_passes(self):
        errors = errors_for(lint.check_public_section_has_public_child, "public_section_no_public_child.yaml")
        self.assertFalse(any("private_section_no_public_child" in e for e in errors))


class TestCheckTextScalarMode(unittest.TestCase):
    def test_valid_schema_produces_no_errors(self):
        errors = lint.check_text_scalar_mode(fixture("valid.yaml"))
        self.assertEqual(errors, [])

    def test_description_with_linebreak_is_error(self):
        errors = lint.check_text_scalar_mode(fixture("text_linebreaks.yaml"))
        self.assertTrue(
            any("desc_with_linebreak" in e or "description" in e.lower() for e in errors),
            f"Expected error for description with \\n, got: {errors}",
        )

    def test_example_with_linebreak_is_error(self):
        errors = lint.check_text_scalar_mode(fixture("text_linebreaks.yaml"))
        self.assertTrue(
            any("example_with_linebreak" in e or "example" in e.lower() for e in errors),
            f"Expected error for example with \\n, got: {errors}",
        )

    def test_title_with_linebreak_is_error(self):
        errors = lint.check_text_scalar_mode(fixture("text_linebreaks.yaml"))
        self.assertTrue(
            any("title_with_linebreak" in e or "title" in e.lower() for e in errors),
            f"Expected error for title with \\n, got: {errors}",
        )

    def test_no_linebreak_passes(self):
        errors = lint.check_text_scalar_mode(fixture("text_linebreaks.yaml"))
        self.assertFalse(any("no_linebreak" in e for e in errors))


class TestCheckRelativeDefaults(unittest.TestCase):
    def test_valid_schema_produces_no_errors(self):
        errors = errors_for(lint.check_relative_defaults, "valid.yaml")
        self.assertEqual(errors, [])

    def test_unknown_variable_is_error(self):
        errors = errors_for(lint.check_relative_defaults, "bad_relative_defaults.yaml")
        self.assertTrue(
            any("unknown_var" in e for e in errors),
            f"Expected error for unknown_var, got: {errors}",
        )

    def test_malformed_placeholder_is_error(self):
        errors = errors_for(lint.check_relative_defaults, "bad_relative_defaults.yaml")
        self.assertTrue(
            any("malformed_placeholder" in e for e in errors),
            f"Expected error for malformed_placeholder, got: {errors}",
        )

    def test_valid_relative_default_passes(self):
        errors = errors_for(lint.check_relative_defaults, "bad_relative_defaults.yaml")
        self.assertFalse(any("valid_relative" in e for e in errors))

    def test_platform_default_with_bad_variable_is_error(self):
        errors = errors_for(lint.check_relative_defaults, "bad_relative_defaults.yaml")
        self.assertTrue(
            any("platform_bad_var" in e for e in errors),
            f"Expected error for platform_bad_var, got: {errors}",
        )

    def test_platform_default_with_malformed_placeholder_is_error(self):
        errors = errors_for(lint.check_relative_defaults, "bad_relative_defaults.yaml")
        self.assertTrue(
            any("platform_malformed" in e for e in errors),
            f"Expected error for platform_malformed, got: {errors}",
        )

    def test_non_relative_default_passes(self):
        errors = errors_for(lint.check_relative_defaults, "bad_relative_defaults.yaml")
        self.assertFalse(any("non_relative" in e for e in errors))

    def test_valid_conf_path_passes(self):
        errors = errors_for(lint.check_relative_defaults, "bad_relative_defaults.yaml")
        self.assertFalse(any("valid_conf_path" in e for e in errors))


class TestCheckEnvParser(unittest.TestCase):
    def test_valid_schema_produces_no_errors(self):
        errors = errors_for(lint.check_env_parser, "valid.yaml")
        self.assertEqual(errors, [])

    def test_invalid_env_parser_value(self):
        errors = errors_for(lint.check_env_parser, "bad_env_parser.yaml")
        self.assertTrue(
            any("bad_value" in e for e in errors),
            f"Expected error for bad_value, got: {errors}",
        )

    def test_env_parser_on_section_is_error(self):
        errors = errors_for(lint.check_env_parser, "bad_env_parser.yaml")
        self.assertTrue(
            any("env_parser_on_section" in e for e in errors),
            f"Expected error for env_parser_on_section, got: {errors}",
        )

    def test_valid_env_parser_values_pass(self):
        errors = errors_for(lint.check_env_parser, "bad_env_parser.yaml")
        for key in ("valid_comma_separated", "valid_space_separated", "valid_json"):
            self.assertFalse(
                any(key in e for e in errors),
                f"Valid setting '{key}' should not produce an error, got: {errors}",
            )

    def test_child_of_bad_section_is_not_flagged(self):
        errors = errors_for(lint.check_env_parser, "bad_env_parser.yaml")
        self.assertFalse(
            any("env_parser_on_section.child" in e for e in errors),
            f"Child setting of bad section should not be flagged, got: {errors}",
        )


class TestCheckRenamedFrom(unittest.TestCase):
    def test_valid_schema_produces_no_errors(self):
        errors = errors_for(lint.check_renamed_from, "valid.yaml")
        self.assertEqual(errors, [])

    def test_list_value_is_error(self):
        # The former format, a plain list of names, no longer carries the deprecation version.
        errors = errors_for(lint.check_renamed_from, "bad_renamed_from.yaml")
        self.assertTrue(
            any("not_a_mapping" in e and "must be a mapping" in e for e in errors),
            f"Expected error for not_a_mapping, got: {errors}",
        )

    def test_empty_mapping_is_error(self):
        errors = errors_for(lint.check_renamed_from, "bad_renamed_from.yaml")
        self.assertTrue(
            any("empty_mapping" in e for e in errors),
            f"Expected error for empty_mapping, got: {errors}",
        )

    def test_blank_former_name_is_error(self):
        errors = errors_for(lint.check_renamed_from, "bad_renamed_from.yaml")
        self.assertTrue(
            any("blank_name" in e for e in errors),
            f"Expected error for blank_name, got: {errors}",
        )

    def test_unquoted_version_is_error(self):
        # An unquoted '7.71' is a YAML float, which would lose the trailing zero of '7.70'.
        errors = errors_for(lint.check_renamed_from, "bad_renamed_from.yaml")
        self.assertTrue(
            any("unquoted_version" in e and "quote the Agent version" in e for e in errors),
            f"Expected error for unquoted_version, got: {errors}",
        )

    def test_partial_version_is_error(self):
        # Versions must be fully semantic: 'MAJOR.MINOR' is not enough.
        errors = errors_for(lint.check_renamed_from, "bad_renamed_from.yaml")
        self.assertTrue(
            any("partial_version" in e and "invalid version" in e for e in errors),
            f"Expected error for partial_version, got: {errors}",
        )

    def test_malformed_version_is_error(self):
        errors = errors_for(lint.check_renamed_from, "bad_renamed_from.yaml")
        self.assertTrue(
            any("malformed_version" in e and "invalid version" in e for e in errors),
            f"Expected error for malformed_version, got: {errors}",
        )

    def test_duplicate_version_is_error(self):
        # Versions order the renames, so a setting cannot be renamed twice in the same version.
        errors = errors_for(lint.check_renamed_from, "bad_renamed_from.yaml")
        duplicates = [e for e in errors if "duplicate_version" in e]
        self.assertEqual(len(duplicates), 1, f"Expected one error for duplicate_version, got: {errors}")
        self.assertIn("reuses version '7.71.0' for 'old_one', 'old_two'", duplicates[0])
        # The third name has its own version and must not be dragged into the error.
        self.assertNotIn("old_three", duplicates[0])

    def test_renamed_from_on_section_is_error(self):
        errors = errors_for(lint.check_renamed_from, "bad_renamed_from.yaml")
        self.assertTrue(
            any("section_with_renamed_from" in e and "only allowed on setting nodes" in e for e in errors),
            f"Expected error for section_with_renamed_from, got: {errors}",
        )

    def test_valid_setting_and_section_pass(self):
        errors = errors_for(lint.check_renamed_from, "bad_renamed_from.yaml")
        for key in ("valid_setting", "valid_section"):
            self.assertFalse(
                any(key in e for e in errors),
                f"Valid node '{key}' should not produce an error, got: {errors}",
            )


if __name__ == "__main__":
    unittest.main()


def product_schema(sku_definitions=None, product_dependencies=None, properties=None):
    """Helper: build an in-memory schema with product enablement definitions."""
    schema = {"properties": properties or {}}
    if sku_definitions is not None:
        schema["sku_definitions"] = sku_definitions
    if product_dependencies is not None:
        schema["product_dependencies"] = product_dependencies
    return schema


class TestCheckProductDefinitions(unittest.TestCase):
    def check(self, schema, is_core=True):
        return lint.check_product_definitions("schema.yaml", schema, is_core)

    def assertErrorContains(self, errors, *fragments):
        self.assertTrue(
            any(all(fragment in e for fragment in fragments) for e in errors),
            f"Expected an error containing {fragments}, got: {errors}",
        )

    def test_valid_definitions_pass(self):
        schema = product_schema(
            sku_definitions={"sku_a": ["product_a", "product_c"]},
            product_dependencies={"product_a": ["product_b", "product_c"], "product_b": ["product_c"], "product_c": []},
        )
        self.assertEqual(self.check(schema), [])

    def test_missing_definitions_pass(self):
        self.assertEqual(self.check(product_schema()), [])

    def test_map_must_be_a_mapping(self):
        errors = self.check(product_schema(sku_definitions=["sku_a"], product_dependencies={}))
        self.assertErrorContains(errors, "sku_definitions", "must be a mapping")

    def test_entry_must_be_a_list_of_strings(self):
        errors = self.check(product_schema(product_dependencies={"product_a": "product_b", "product_b": [1]}))
        self.assertErrorContains(errors, "product_dependencies.product_a", "list of product names")
        self.assertErrorContains(errors, "product_dependencies.product_b", "list of product names")

    def test_name_in_both_maps(self):
        errors = self.check(product_schema(sku_definitions={"shared": []}, product_dependencies={"shared": []}))
        self.assertErrorContains(errors, "'shared'", "both a SKU and a product")

    def test_sku_references_undeclared_product(self):
        errors = self.check(product_schema(sku_definitions={"sku_a": ["missing"]}, product_dependencies={}))
        self.assertErrorContains(errors, "sku_definitions.sku_a", "'missing'", "not declared")

    def test_dependency_references_undeclared_product(self):
        errors = self.check(product_schema(product_dependencies={"product_a": ["missing"]}))
        self.assertErrorContains(errors, "product_dependencies.product_a", "'missing'", "not declared")

    def test_product_depending_on_a_sku(self):
        errors = self.check(
            product_schema(sku_definitions={"sku_a": []}, product_dependencies={"product_a": ["sku_a"]})
        )
        self.assertErrorContains(errors, "product_dependencies.product_a", "'sku_a'", "is a SKU")

    def test_sku_bundling_a_sku(self):
        errors = self.check(product_schema(sku_definitions={"sku_a": ["sku_b"], "sku_b": []}, product_dependencies={}))
        self.assertErrorContains(errors, "sku_definitions.sku_a", "'sku_b'", "is a SKU")

    def test_dependency_cycle(self):
        errors = self.check(
            product_schema(
                product_dependencies={
                    "product_a": ["product_b"],
                    "product_b": ["product_c"],
                    "product_c": ["product_a"],
                }
            )
        )
        cycles = [e for e in errors if "cycle" in e]
        self.assertEqual(len(cycles), 1, errors)
        self.assertIn("product_a -> product_b -> product_c -> product_a", cycles[0])

    def test_self_dependency_is_a_cycle(self):
        errors = self.check(product_schema(product_dependencies={"product_a": ["product_a"]}))
        self.assertErrorContains(errors, "cycle", "product_a -> product_a")

    def test_definitions_outside_core_schema(self):
        errors = self.check(product_schema(sku_definitions={}, product_dependencies={}), is_core=False)
        self.assertErrorContains(errors, "'sku_definitions'", "core schema")
        self.assertErrorContains(errors, "'product_dependencies'", "core schema")


class TestCheckProductDefaults(unittest.TestCase):
    DECLARED = {"product_a", "product_b"}

    def check(self, properties, declared=None):
        schema = product_schema(properties=properties)
        return lint.check_product_defaults("schema.yaml", schema, self.DECLARED if declared is None else declared)

    def assertErrorContains(self, errors, *fragments):
        self.assertTrue(
            any(all(fragment in e for fragment in fragments) for e in errors),
            f"Expected an error containing {fragments}, got: {errors}",
        )

    def test_valid_product_defaults_pass(self):
        properties = {
            "logs_enabled": {
                "node_type": "setting",
                "type": "boolean",
                "default": False,
                "product_defaults": {"product_a": True, "product_b": False},
            },
            "logs_config": {
                "node_type": "section",
                "properties": {
                    "container_collect_all": {
                        "node_type": "setting",
                        "type": "boolean",
                        "default": False,
                        # 'other' is not required: a missing platform means no product default
                        "product_platform_defaults": {"product_a": {"container": True, "fargate": True}},
                    },
                    "tags": {
                        "node_type": "setting",
                        "type": "array",
                        "items": {"type": "string"},
                        "default": [],
                        "product_defaults": {"product_a": ["a:b"]},
                    },
                },
            },
        }
        self.assertEqual(self.check(properties), [])

    def test_both_keywords_on_one_setting(self):
        errors = self.check(
            {
                "x": {
                    "node_type": "setting",
                    "type": "boolean",
                    "default": False,
                    "product_defaults": {"product_a": True},
                    "product_platform_defaults": {"product_b": {"linux": True}},
                }
            }
        )
        self.assertErrorContains(errors, "[x]", "mutually exclusive")

    def test_keyword_on_section(self):
        errors = self.check(
            {
                "section": {
                    "node_type": "section",
                    "product_defaults": {"product_a": {}},
                    "properties": {"x": {"node_type": "setting", "type": "boolean", "default": False}},
                }
            }
        )
        self.assertErrorContains(errors, "[section]", "only be used on settings")

    def test_undeclared_product(self):
        errors = self.check(
            {
                "x": {
                    "node_type": "setting",
                    "type": "boolean",
                    "default": False,
                    "product_defaults": {"missing": True},
                },
                "y": {
                    "node_type": "setting",
                    "type": "boolean",
                    "default": False,
                    "product_platform_defaults": {"missing_too": {"linux": True}},
                },
            }
        )
        self.assertErrorContains(errors, "[x]", "'missing'", "not declared")
        self.assertErrorContains(errors, "[y]", "'missing_too'", "not declared")

    def test_keyword_must_be_a_mapping(self):
        errors = self.check(
            {
                "x": {"node_type": "setting", "type": "boolean", "default": False, "product_defaults": [True]},
                "y": {
                    "node_type": "setting",
                    "type": "boolean",
                    "default": False,
                    "product_platform_defaults": {"product_a": True},
                },
            }
        )
        self.assertErrorContains(errors, "[x]", "'product_defaults' must be a mapping")
        self.assertErrorContains(errors, "[y]", "product_a", "mapping of platforms")

    def test_invalid_platform_key(self):
        errors = self.check(
            {
                "x": {
                    "node_type": "setting",
                    "type": "boolean",
                    "default": False,
                    "product_platform_defaults": {"product_a": {"linux": True, "solaris": True}},
                }
            }
        )
        self.assertErrorContains(errors, "[x]", "'solaris'")

    def test_value_type_mismatch(self):
        errors = self.check(
            {
                "flag": {
                    "node_type": "setting",
                    "type": "boolean",
                    "default": False,
                    "product_defaults": {"product_a": "yes"},
                },
                "count": {
                    "node_type": "setting",
                    "type": "integer",
                    "default": 0,
                    "product_defaults": {"product_a": True},
                },
                "ratio": {
                    "node_type": "setting",
                    "type": "number",
                    "default": 0,
                    "product_platform_defaults": {"product_a": {"linux": "high"}},
                },
            }
        )
        self.assertErrorContains(errors, "[flag]", "product_a", "'boolean'")
        self.assertErrorContains(errors, "[count]", "product_a", "'integer'")
        self.assertErrorContains(errors, "[ratio]", "product_a", "linux", "'number'")

    def test_number_accepts_integers(self):
        properties = {
            "ratio": {"node_type": "setting", "type": "number", "default": 0.5, "product_defaults": {"product_a": 1}}
        }
        self.assertEqual(self.check(properties), [])


class TestCheckProductConflicts(unittest.TestCase):
    def check(self, sku_definitions, product_dependencies, properties):
        schema = product_schema(properties=properties)
        return lint.check_product_conflicts("schema.yaml", schema, sku_definitions, product_dependencies)

    @staticmethod
    def setting(**keywords):
        return {"node_type": "setting", "type": "boolean", "default": False, **keywords}

    def test_sku_closure_conflict(self):
        errors = self.check(
            {"sku_a": ["product_a", "product_b"]},
            {"product_a": [], "product_b": []},
            {"x": self.setting(product_defaults={"product_a": True, "product_b": False})},
        )
        self.assertEqual(len(errors), 1, errors)
        for fragment in ("[x]", "SKU 'sku_a'", "product_a=True", "product_b=False", "linux"):
            self.assertIn(fragment, errors[0])

    def test_product_dependency_closure_conflict(self):
        errors = self.check(
            {},
            {"product_a": ["product_b"], "product_b": ["product_c"], "product_c": []},
            {"x": self.setting(product_defaults={"product_a": True, "product_c": False})},
        )
        self.assertEqual(len(errors), 1, errors)
        for fragment in ("[x]", "product 'product_a'", "product_a=True", "product_c=False"):
            self.assertIn(fragment, errors[0])

    def test_conflicting_products_not_bundled_together(self):
        errors = self.check(
            {"sku_a": ["product_a"], "sku_b": ["product_b"]},
            {"product_a": [], "product_b": []},
            {"x": self.setting(product_defaults={"product_a": True, "product_b": False})},
        )
        self.assertEqual(errors, [])

    def test_same_value_from_two_products(self):
        errors = self.check(
            {"sku_a": ["product_a", "product_b"]},
            {"product_a": [], "product_b": []},
            {"x": self.setting(product_defaults={"product_a": True, "product_b": True})},
        )
        self.assertEqual(errors, [])

    def test_values_compared_with_their_type(self):
        # 1 and True are equal in Python but not for the Agent: they are different values
        errors = self.check(
            {"sku_a": ["product_a", "product_b"]},
            {"product_a": [], "product_b": []},
            {"x": {"node_type": "setting", "default": 0, "product_defaults": {"product_a": 1, "product_b": True}}},
        )
        self.assertEqual(len(errors), 1, errors)

    def test_platform_specific_conflict(self):
        errors = self.check(
            {"sku_a": ["product_a", "product_b"]},
            {"product_a": [], "product_b": []},
            {
                "x": self.setting(
                    product_platform_defaults={"product_a": {"windows": True}, "product_b": {"windows": False}}
                )
            },
        )
        self.assertEqual(len(errors), 1, errors)
        self.assertIn("windows", errors[0])
        self.assertNotIn("linux", errors[0])

    def test_runtime_fallback_conflict(self):
        # On a Linux container the runtime falls back container -> linux: product_a gives True (container)
        # and product_b gives False (linux). Checking each platform key on its own would miss it.
        errors = self.check(
            {"sku_a": ["product_a", "product_b"]},
            {"product_a": [], "product_b": []},
            {
                "x": self.setting(
                    product_platform_defaults={"product_a": {"container": True}, "product_b": {"linux": False}}
                )
            },
        )
        self.assertEqual(len(errors), 1, errors)
        self.assertIn("linux/container", errors[0])
        self.assertNotIn("windows", errors[0])

    def test_product_defaults_apply_to_every_platform(self):
        # product_a sets the value everywhere, product_b only through the 'other' fallback: they disagree on
        # every environment
        errors = self.check(
            {"sku_a": ["product_a", "product_b"]},
            {"product_a": [], "product_b": []},
            {
                "x": {
                    "node_type": "setting",
                    "type": "boolean",
                    "default": False,
                    "product_defaults": {"product_a": True},
                },
                "y": self.setting(
                    product_platform_defaults={"product_a": {"linux": True}, "product_b": {"other": False}}
                ),
            },
        )
        self.assertEqual(len(errors), 1, errors)
        self.assertIn("[y]", errors[0])
        self.assertIn("linux", errors[0])

    def test_nested_settings_are_checked(self):
        errors = self.check(
            {"sku_a": ["product_a", "product_b"]},
            {"product_a": [], "product_b": []},
            {
                "section": {
                    "node_type": "section",
                    "properties": {"x": self.setting(product_defaults={"product_a": True, "product_b": False})},
                }
            },
        )
        self.assertEqual(len(errors), 1, errors)
        self.assertIn("[section.x]", errors[0])


class TestProductPlatformKubernetes(unittest.TestCase):
    def test_kubernetes_is_a_valid_platform_key(self):
        schema = product_schema(
            properties={
                "x": {
                    "node_type": "setting",
                    "type": "boolean",
                    "default": False,
                    "product_platform_defaults": {"product_a": {"kubernetes": True}},
                }
            }
        )
        self.assertEqual(lint.check_product_defaults("schema.yaml", schema, {"product_a"}), [])

    def test_kubernetes_falls_back_to_container(self):
        # On Kubernetes, product_a resolves through 'container' and product_b through 'kubernetes': they conflict
        properties = {
            "x": {
                "node_type": "setting",
                "type": "boolean",
                "default": False,
                "product_platform_defaults": {"product_a": {"container": True}, "product_b": {"kubernetes": False}},
            }
        }
        errors = lint.check_product_conflicts(
            "schema.yaml",
            product_schema(properties=properties),
            {"sku_a": ["product_a", "product_b"]},
            {"product_a": [], "product_b": []},
        )
        self.assertEqual(len(errors), 1, errors)
        self.assertIn("linux/kubernetes", errors[0])
        self.assertNotIn("linux/container,", errors[0])
