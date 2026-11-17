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


class TestCheckSensitive(unittest.TestCase):
    def test_valid_schema_produces_no_errors(self):
        errors = errors_for(lint.check_sensitive, "valid.yaml")
        self.assertEqual(errors, [])

    def test_non_boolean_is_error(self):
        errors = errors_for(lint.check_sensitive, "bad_sensitive.yaml")
        self.assertTrue(any("not_bool" in e for e in errors), f"Expected error for not_bool, got: {errors}")

    def test_sensitive_on_section_is_error(self):
        errors = errors_for(lint.check_sensitive, "bad_sensitive.yaml")
        self.assertTrue(
            any("sensitive_on_section" in e for e in errors),
            f"Expected error for sensitive_on_section, got: {errors}",
        )

    def test_valid_values_pass(self):
        errors = errors_for(lint.check_sensitive, "bad_sensitive.yaml")
        for key in ("valid_sensitive", "valid_not_sensitive", "sensitive_on_section.child"):
            self.assertFalse(any(key in e for e in errors), f"'{key}' should not produce an error, got: {errors}")


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


class TestCheckSnakeCaseNames(unittest.TestCase):
    def test_valid_schema_produces_no_errors(self):
        errors = errors_for(lint.check_snake_case_names, "valid.yaml")
        self.assertEqual(errors, [])

    def test_non_snake_case_names_are_errors(self):
        errors = errors_for(lint.check_snake_case_names, "bad_snake_case.yaml")
        for path in (
            "camelCase",
            "UPPER_prefix",
            "double__underscore",
            "_leading_underscore",
            "trailing_underscore_",
            "kebab-case",
            "BadSection",
            "valid_section.nestedBad",
        ):
            self.assertTrue(
                any(f"[{path}]" in e and "is not snake_case" in e for e in errors),
                f"Expected error for {path}, got: {errors}",
            )

    def test_only_offending_name_is_reported(self):
        # A non snake_case section does not make its snake_case children fail.
        errors = errors_for(lint.check_snake_case_names, "bad_snake_case.yaml")
        self.assertFalse(any("good_child" in e for e in errors), f"Unexpected error for good_child, got: {errors}")

    def test_valid_names_pass(self):
        errors = errors_for(lint.check_snake_case_names, "bad_snake_case.yaml")
        for path in ("[valid_section]", "valid_v2_setting", "[valid_setting]"):
            self.assertFalse(any(path in e for e in errors), f"Unexpected error for {path}, got: {errors}")

    def test_excepted_names_pass(self):
        errors = errors_for(
            lint.check_snake_case_names, "bad_snake_case.yaml", {"camelCase", "valid_section.nestedBad"}
        )
        self.assertFalse(any("camelCase" in e or "nestedBad" in e for e in errors), f"Got: {errors}")
        self.assertTrue(any("UPPER_prefix" in e for e in errors))


class TestCheckTemplateSectionOnPublic(unittest.TestCase):
    def test_valid_schema_produces_no_errors(self):
        errors = errors_for(lint.check_template_section_on_public, "valid.yaml")
        self.assertEqual(errors, [])

    def test_private_setting_with_template_section_is_error(self):
        errors = errors_for(lint.check_template_section_on_public, "bad_template_section.yaml")
        self.assertTrue(
            any("private_setting" in e and "only valid on public nodes" in e for e in errors),
            f"Expected error for private_setting, got: {errors}",
        )

    def test_private_section_with_template_section_is_error(self):
        errors = errors_for(lint.check_template_section_on_public, "bad_template_section.yaml")
        self.assertTrue(
            any("[private_section]" in e for e in errors),
            f"Expected error for private_section, got: {errors}",
        )

    def test_public_setting_with_template_section_passes(self):
        errors = errors_for(lint.check_template_section_on_public, "bad_template_section.yaml")
        self.assertFalse(
            any("public_setting" in e for e in errors),
            f"public_setting should not produce an error, got: {errors}",
        )


class TestCheckNoNullValues(unittest.TestCase):
    def test_valid_schema_produces_no_errors(self):
        errors = errors_for(lint.check_no_null_values, "valid.yaml")
        self.assertEqual(errors, [])

    def test_every_way_of_accepting_null_is_an_error(self):
        errors = errors_for(lint.check_no_null_values, "bad_null_value.yaml")
        for location, reason in (
            ("[enum_with_null]", "'enum' lists null"),
            ("[const_null]", "'const' is null"),
            ("[nullable_type]", "'type' accepts null"),
            ("[nested_items_null.items.properties.mode]", "'enum' lists null"),
            ("[any_of_null.anyOf[1]]", "'type' accepts null"),
            ("[section.nested_enum_with_null]", "'enum' lists null"),
        ):
            self.assertTrue(
                any(location in e and reason in e for e in errors),
                f"Expected {reason!r} for {location}, got: {errors}",
            )
        self.assertEqual(len(errors), 6, errors)

    def test_null_in_instance_data_is_not_reported(self):
        errors = errors_for(lint.check_no_null_values, "bad_null_value.yaml")
        self.assertFalse(any("null_in_default_object" in e or "valid_enum" in e for e in errors), f"Got: {errors}")

    def test_excepted_settings_pass(self):
        errors = errors_for(
            lint.check_no_null_values, "bad_null_value.yaml", {"enum_with_null", "section.nested_enum_with_null"}
        )
        self.assertFalse(any("[enum_with_null]" in e or "nested_enum_with_null" in e for e in errors), f"Got: {errors}")
        self.assertTrue(any("[const_null]" in e for e in errors))

    def test_no_new_null_exception(self):
        """No setting may be added to the null_value exceptions, see check_no_null_values in lint.py.

        Only removing entries is allowed: when an exception is fixed, remove it from both
        lint_exceptions.yaml and this list.
        """
        allowed = {"infrastructure_mode", "logs_config.file_wildcard_selection_mode"}
        exceptions = lint.load_exceptions()["null_value"]
        self.assertLessEqual(
            exceptions,
            allowed,
            f"New null_value exception(s) {sorted(exceptions - allowed)}: remove null from the setting instead.",
        )


class TestCheckNoTitleOnSettings(unittest.TestCase):
    def test_valid_schema_produces_no_errors(self):
        errors = errors_for(lint.check_no_title_on_settings, "valid.yaml")
        self.assertEqual(errors, [])

    def test_settings_with_title_are_errors_pointing_to_banner(self):
        errors = errors_for(lint.check_no_title_on_settings, "bad_title_on_setting.yaml")
        self.assertEqual(len(errors), 2, errors)
        for path in ("[setting_with_title]", "[section_with_title.nested_setting_with_title]"):
            self.assertTrue(
                any(path in e and "rename 'title' to 'banner'" in e for e in errors),
                f"Expected error for {path}, got: {errors}",
            )

    def test_banner_on_setting_and_title_on_section_pass(self):
        errors = errors_for(lint.check_no_title_on_settings, "bad_title_on_setting.yaml")
        self.assertFalse(
            any("[setting_with_banner]" in e or "[section_with_title]" in e for e in errors), f"Got: {errors}"
        )


class TestCheckFormats(unittest.TestCase):
    def test_valid_schema_produces_no_errors(self):
        errors = errors_for(lint.check_formats, "valid.yaml")
        self.assertEqual(errors, [])

    def test_invalid_formats_are_errors(self):
        errors = errors_for(lint.check_formats, "bad_format.yaml")
        for location, message in (
            ("[unknown_format]", "Unknown format 'uri'"),
            ("[format_on_integer]", "only applies to strings"),
            ("[nested_unknown_format.items]", "Unknown format 'hostname'"),
        ):
            self.assertTrue(
                any(location in e and message in e for e in errors), f"Expected {message!r} for {location}: {errors}"
            )
        self.assertEqual(len(errors), 3, errors)

    def test_agent_formats_and_instance_data_pass(self):
        errors = errors_for(lint.check_formats, "bad_format.yaml")
        for name in ("valid_url", "valid_duration", "format_in_default"):
            self.assertFalse(any(f"[{name}]" in e for e in errors), f"Unexpected error for {name}: {errors}")


if __name__ == "__main__":
    unittest.main()
