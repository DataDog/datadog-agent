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


LOGS = {
    "dependencies": [],
    "profiles": {
        "logs_high_concurrency": [],
        "logs_high_throughput": ["logs_high_concurrency"],
        "logs_low_resource": {"dependencies": [], "conflict": ["logs_high_throughput"]},
    },
}


class TestCheckProductDefinitions(unittest.TestCase):
    def check(self, schema, is_core=True):
        return lint.check_product_definitions("schema.yaml", schema, is_core)

    def assertErrorContains(self, errors, *fragments):
        self.assertTrue(
            any(all(fragment in e for fragment in fragments) for e in errors),
            f"Expected an error containing {fragments}, got: {errors}",
        )

    def valid(self, **extra):
        definitions = {
            "logs": LOGS,
            "apm": {"dependencies": [], "conflict": ["error_tracking"]},
            "error_tracking": {"dependencies": []},
            "siem": {"dependencies": ["logs"]},
        }
        definitions.update(extra)
        return definitions

    def test_valid_definitions_pass(self):
        schema = product_schema(sku_definitions={"sku_a": ["apm", "siem"]}, product_dependencies=self.valid())
        self.assertEqual(self.check(schema), [])

    def test_missing_definitions_pass(self):
        self.assertEqual(self.check(product_schema()), [])

    def test_maps_must_be_mappings(self):
        errors = self.check(product_schema(sku_definitions=["sku_a"], product_dependencies=["apm"]))
        self.assertErrorContains(errors, "'sku_definitions'", "must be a mapping")
        self.assertErrorContains(errors, "'product_dependencies'", "must be a mapping")

    def test_product_must_be_a_mapping(self):
        errors = self.check(product_schema(product_dependencies={"apm": []}))
        self.assertErrorContains(errors, "[product_dependencies.apm]", "must be a mapping")

    def test_unknown_product_key(self):
        errors = self.check(product_schema(product_dependencies={"apm": {"dependencies": [], "depends": []}}))
        self.assertErrorContains(errors, "[product_dependencies.apm]", "unknown key", "'depends'")

    def test_lists_of_names(self):
        errors = self.check(product_schema(product_dependencies={"apm": {"dependencies": "logs", "conflict": [1]}}))
        self.assertErrorContains(errors, "[product_dependencies.apm.dependencies]", "list of names")
        self.assertErrorContains(errors, "[product_dependencies.apm.conflict]", "list of names")

    def test_profile_forms(self):
        errors = self.check(
            product_schema(
                product_dependencies={
                    "logs": {"dependencies": [], "profiles": {"p1": "x", "p2": {"dependencies": [], "dependency": []}}}
                }
            )
        )
        self.assertErrorContains(errors, "[product_dependencies.logs.profiles.p1]", "list of names or a mapping")
        self.assertErrorContains(errors, "[product_dependencies.logs.profiles.p2]", "unknown key", "'dependency'")

    def test_names_are_unique(self):
        errors = self.check(
            product_schema(
                sku_definitions={"apm": []},
                product_dependencies=self.valid(
                    other={"dependencies": [], "profiles": {"logs_high_concurrency": []}},
                    logs_low_resource_product={"dependencies": []},
                ),
            )
        )
        self.assertErrorContains(errors, "'apm'", "both a SKU and a product")
        self.assertErrorContains(errors, "'logs_high_concurrency'", "declared more than once")

    def test_profile_named_like_a_product(self):
        errors = self.check(
            product_schema(
                product_dependencies={
                    "apm": {"dependencies": []},
                    "logs": {"dependencies": [], "profiles": {"apm": []}},
                }
            )
        )
        self.assertErrorContains(errors, "'apm'", "declared more than once")

    def test_undeclared_references(self):
        errors = self.check(
            product_schema(
                sku_definitions={"sku_a": ["missing_1"]},
                product_dependencies={
                    "apm": {"dependencies": ["missing_2"], "conflict": ["missing_3"], "profiles": {"p": ["missing_4"]}}
                },
            )
        )
        for name in ("missing_1", "missing_2", "missing_3", "missing_4"):
            self.assertErrorContains(errors, f"'{name}'", "not declared")

    def test_skus_bundle_products_only(self):
        errors = self.check(
            product_schema(
                sku_definitions={"sku_a": ["logs_high_throughput"], "sku_b": ["sku_a"]},
                product_dependencies=self.valid(),
            )
        )
        self.assertErrorContains(errors, "[sku_definitions.sku_a]", "'logs_high_throughput' is a profile")
        self.assertErrorContains(errors, "[sku_definitions.sku_b]", "'sku_a' is a SKU")

    def test_dependency_on_a_sku(self):
        errors = self.check(
            product_schema(sku_definitions={"sku_a": []}, product_dependencies={"apm": {"dependencies": ["sku_a"]}})
        )
        self.assertErrorContains(errors, "[product_dependencies.apm.dependencies]", "'sku_a' is a SKU")

    def test_profiles_can_depend_on_profiles_of_other_products(self):
        definitions = self.valid(apm={"dependencies": [], "profiles": {"apm_fast": ["logs_high_throughput"]}})
        self.assertEqual(self.check(product_schema(product_dependencies=definitions)), [])

    def test_cycle_through_profiles(self):
        definitions = {
            "a": {"dependencies": [], "profiles": {"a_p": ["b_p"]}},
            "b": {"dependencies": [], "profiles": {"b_p": ["a_p"]}},
        }
        errors = self.check(product_schema(product_dependencies=definitions))
        self.assertErrorContains(errors, "cycle", "a_p -> b_p -> a_p")

    def test_conflict_with_itself(self):
        errors = self.check(product_schema(product_dependencies={"apm": {"dependencies": [], "conflict": ["apm"]}}))
        self.assertErrorContains(errors, "'apm'", "conflicts with itself")

    def test_conflict_within_dependencies(self):
        definitions = self.valid(siem={"dependencies": ["logs", "apm", "error_tracking"]})
        errors = self.check(product_schema(product_dependencies=definitions))
        self.assertErrorContains(errors, "'siem'", "enables 'apm' and 'error_tracking'", "conflict")

    def test_conflict_within_sku(self):
        errors = self.check(
            product_schema(sku_definitions={"sku_a": ["apm", "error_tracking"]}, product_dependencies=self.valid())
        )
        self.assertErrorContains(errors, "SKU 'sku_a'", "enables 'apm' and 'error_tracking'")

    def test_definitions_outside_core_schema(self):
        errors = self.check(product_schema(sku_definitions={}, product_dependencies={}), is_core=False)
        self.assertErrorContains(errors, "'sku_definitions'", "core schema")
        self.assertErrorContains(errors, "'product_dependencies'", "core schema")


class TestProductCatalog(unittest.TestCase):
    def test_flattened_dependencies(self):
        schema = product_schema(
            sku_definitions={"sku_a": ["siem"]}, product_dependencies={"logs": LOGS, "siem": {"dependencies": ["logs"]}}
        )
        sku_definitions, dependencies = lint.get_product_definitions(schema)
        self.assertEqual(sku_definitions, {"sku_a": ["siem"]})
        # a profile depends on its product, plus its own dependencies
        self.assertEqual(sorted(dependencies["logs_high_throughput"]), ["logs", "logs_high_concurrency"])
        self.assertEqual(dependencies["logs"], [])
        self.assertEqual(dependencies["siem"], ["logs"])
        self.assertEqual(
            lint.resolve_product_closure(["logs_high_throughput"], dependencies),
            {"logs", "logs_high_concurrency", "logs_high_throughput"},
        )

    def test_conflicts_are_symmetric(self):
        conflicts = lint.get_product_conflicts(product_schema(product_dependencies={"logs": LOGS}))
        self.assertEqual(conflicts["logs_low_resource"], {"logs_high_throughput"})
        self.assertEqual(conflicts["logs_high_throughput"], {"logs_low_resource"})

    def test_profile_owners(self):
        owners = lint.get_profile_owners(product_schema(product_dependencies={"logs": LOGS}))
        self.assertEqual(
            owners, {"logs_high_concurrency": "logs", "logs_high_throughput": "logs", "logs_low_resource": "logs"}
        )


class TestNullPlatformValue(unittest.TestCase):
    def setting(self, platforms):
        return {"node_type": "setting", "type": "boolean", "default": False, "product_platform_defaults": platforms}

    def test_null_is_valid(self):
        schema = product_schema(properties={"x": self.setting({"product_a": {"linux": True, "container": None}})})
        self.assertEqual(lint.check_product_defaults("schema.yaml", schema, {"product_a"}), [])

    def test_null_stops_the_fallback(self):
        # product_a sets nothing in containers (no fallback to linux), product_b sets true there: no conflict
        schema = product_schema(
            properties={
                "x": self.setting({"product_a": {"linux": True, "container": None}, "product_b": {"container": False}})
            }
        )
        errors = lint.check_product_conflicts(
            "schema.yaml", schema, {"sku_a": ["product_a", "product_b"]}, {"product_a": [], "product_b": []}
        )
        self.assertEqual(errors, [])
        self.assertEqual(
            lint._product_value(schema["properties"]["x"], "product_a", ["container", "linux", "other"]), (False, None)
        )


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


class TestProductPlatformClusterAgent(unittest.TestCase):
    def test_cluster_agent_is_a_valid_platform_key(self):
        schema = product_schema(
            properties={
                "x": {
                    "node_type": "setting",
                    "type": "array",
                    "items": {"type": "string"},
                    "default": [],
                    "product_platform_defaults": {"product_a": {"cluster_agent": ["a"], "kubernetes": ["b"]}},
                }
            }
        )
        self.assertEqual(lint.check_product_defaults("schema.yaml", schema, {"product_a"}), [])

    def test_cluster_agent_falls_back_to_kubernetes(self):
        # On the Cluster Agent, product_a resolves through 'kubernetes' and product_b through 'cluster_agent'
        properties = {
            "x": {
                "node_type": "setting",
                "type": "boolean",
                "default": False,
                "product_platform_defaults": {"product_a": {"kubernetes": True}, "product_b": {"cluster_agent": False}},
            }
        }
        errors = lint.check_product_conflicts(
            "schema.yaml",
            product_schema(properties=properties),
            {"sku_a": ["product_a", "product_b"]},
            {"product_a": [], "product_b": []},
        )
        self.assertEqual(len(errors), 1, errors)
        self.assertIn("linux/cluster_agent", errors[0])


class TestProductCombinations(unittest.TestCase):
    NODE = {
        "node_type": "setting",
        "type": "integer",
        "default": 10,
        "product_defaults": {
            "p1": 21,
            "p2": 100,
            "p1+p2": 75,
            "p3": 50,
            "p1+p2+p3": 90,
            "p4": 100,
            "p2+p4": 60,
        },
    }
    KEYS = ["linux", "other"]

    def values(self, enabled):
        return lint.product_setting_values(self.NODE, enabled, self.KEYS)

    def test_single_products(self):
        self.assertEqual(self.values(["p1"]), [("p1", 21)])

    def test_combination_replaces_its_products(self):
        self.assertEqual(self.values(["p1", "p2"]), [("p1+p2", 75)])

    def test_most_specific_combination_wins(self):
        self.assertEqual(self.values(["p1", "p2", "p3"]), [("p1+p2+p3", 90)])

    def test_products_outside_the_combination_still_count(self):
        self.assertEqual(self.values(["p1", "p3"]), [("p1", 21), ("p3", 50)])

    def test_overlapping_combinations(self):
        self.assertEqual(self.values(["p1", "p2", "p4"]), [("p1+p2", 75), ("p2+p4", 60)])

    def test_null_combination_value(self):
        node = {
            "product_platform_defaults": {
                "p1": {"other": 1},
                "p2": {"other": 2},
                "p1+p2": {"container": None, "other": 3},
            }
        }
        self.assertEqual(lint.product_setting_values(node, ["p1", "p2"], ["linux", "other"]), [("p1+p2", 3)])
        self.assertEqual(
            lint.product_setting_values(node, ["p1", "p2"], ["container", "linux", "other"]), [("p1", 1), ("p2", 2)]
        )

    def test_closure_conflicts_use_combinations(self):
        schema = product_schema(properties={"a": self.NODE})
        dependencies = {"p1": [], "p2": [], "p3": [], "p4": []}
        # p1 and p2 conflict (21 vs 100) but the combination resolves it
        self.assertEqual(lint.check_product_conflicts("schema.yaml", schema, {"sku_a": ["p1", "p2"]}, dependencies), [])
        errors = lint.check_product_conflicts("schema.yaml", schema, {"sku_b": ["p1", "p3"]}, dependencies)
        self.assertEqual(len(errors), 1, errors)
        self.assertIn("p1=21, p3=50", errors[0])


class TestCombinationKeyValidation(unittest.TestCase):
    DECLARED = {"p1", "p2", "p3", "logs_fast", "logs_small"}
    CONFLICTS = {"p1": {"p3"}, "p3": {"p1"}}
    OWNERS = {"logs_fast": "logs", "logs_small": "logs"}

    def check(self, product_defaults):
        schema = product_schema(
            properties={
                "a": {"node_type": "setting", "type": "integer", "default": 0, "product_defaults": product_defaults}
            }
        )
        return lint.check_product_defaults("schema.yaml", schema, self.DECLARED, self.CONFLICTS, self.OWNERS)

    def assertErrorContains(self, errors, *fragments):
        self.assertTrue(
            any(all(fragment in e for fragment in fragments) for e in errors),
            f"Expected an error containing {fragments}, got: {errors}",
        )

    def test_valid_combination(self):
        self.assertEqual(self.check({"p1": 1, "p2": 2, "p1+p2": 3, "p2+logs_fast": 4}), [])

    def test_undeclared_name_in_combination(self):
        self.assertErrorContains(self.check({"p1+missing": 1}), "[a]", "'missing'", "not declared")

    def test_repeated_name(self):
        self.assertErrorContains(self.check({"p1+p1": 1}), "[a]", "'p1+p1'", "more than once")

    def test_same_combination_twice(self):
        self.assertErrorContains(self.check({"p1+p2": 1, "p2+p1": 2}), "[a]", "'p1+p2'", "'p2+p1'", "same combination")

    def test_conflicting_names(self):
        self.assertErrorContains(self.check({"p1+p3": 1}), "[a]", "'p1+p3'", "conflict", "never")

    def test_two_profiles_of_a_product(self):
        self.assertErrorContains(self.check({"logs_fast+logs_small": 1}), "[a]", "two profiles of 'logs'")
