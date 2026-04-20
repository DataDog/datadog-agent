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
        errors = errors_for(lint.check_settings_have_default, "valid.yaml", set())
        self.assertEqual(errors, [])

    def test_setting_without_default(self):
        errors = errors_for(lint.check_settings_have_default, "missing_default.yaml", set())
        self.assertTrue(any("no_default" in e for e in errors), f"Expected error for no_default, got: {errors}")

    def test_excepted_setting_with_tag_passes(self):
        errors = errors_for(lint.check_settings_have_default, "missing_default.yaml", {"excepted_no_default"})
        self.assertFalse(
            any("excepted_no_default" in e for e in errors),
            f"Excepted setting should not produce an error, got: {errors}",
        )

    def test_excepted_setting_without_tag_is_still_an_error(self):
        # A setting in the exception list but without the required tag is an error
        errors = errors_for(lint.check_settings_have_default, "missing_default.yaml", {"no_default"})
        self.assertTrue(
            any("no_default" in e and "TODO:fix-no-default" in e for e in errors),
            f"Expected tag-enforcement error for no_default, got: {errors}",
        )

    def test_setting_with_platform_default_passes(self):
        errors = errors_for(lint.check_settings_have_default, "missing_default.yaml", set())
        self.assertFalse(any("with_platform_default" in e for e in errors))


class TestCheckSettingsHaveType(unittest.TestCase):
    def test_valid_schema_produces_no_errors(self):
        errors = errors_for(lint.check_settings_have_type, "valid.yaml", set())
        self.assertEqual(errors, [])

    def test_setting_without_type(self):
        errors = errors_for(lint.check_settings_have_type, "missing_type.yaml", set())
        self.assertTrue(any("no_type" in e for e in errors), f"Expected error for no_type, got: {errors}")

    def test_excepted_setting_with_tag_passes(self):
        errors = errors_for(lint.check_settings_have_type, "missing_type.yaml", {"excepted_no_type"})
        self.assertFalse(
            any("excepted_no_type" in e for e in errors),
            f"Excepted setting should not produce an error, got: {errors}",
        )

    def test_excepted_setting_without_tag_is_still_an_error(self):
        errors = errors_for(lint.check_settings_have_type, "missing_type.yaml", {"no_type"})
        self.assertTrue(
            any("no_type" in e and "TODO:fix-missing-type" in e for e in errors),
            f"Expected tag-enforcement error for no_type, got: {errors}",
        )


class TestCheckPlatformDefaultKeys(unittest.TestCase):
    def test_valid_schema_produces_no_errors(self):
        errors = errors_for(lint.check_platform_default_keys, "valid.yaml")
        self.assertEqual(errors, [])

    def test_missing_required_platform_key(self):
        errors = errors_for(lint.check_platform_default_keys, "bad_platform_default.yaml")
        self.assertTrue(
            any("missing_windows" in e for e in errors), f"Expected error for missing_windows, got: {errors}"
        )

    def test_unknown_platform_key(self):
        errors = errors_for(lint.check_platform_default_keys, "bad_platform_default.yaml")
        self.assertTrue(any("unknown_key" in e for e in errors), f"Expected error for unknown_key, got: {errors}")

    def test_valid_with_other_passes(self):
        errors = errors_for(lint.check_platform_default_keys, "bad_platform_default.yaml")
        self.assertFalse(any("valid_with_other" in e for e in errors))

    def test_valid_all_platforms_passes(self):
        errors = errors_for(lint.check_platform_default_keys, "bad_platform_default.yaml")
        self.assertFalse(any("valid_all_platforms" in e for e in errors))


if __name__ == "__main__":
    unittest.main()
