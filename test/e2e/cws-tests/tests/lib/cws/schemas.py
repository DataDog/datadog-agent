import json
import os

from jsonschema import Draft7Validator, RefResolver


class JsonSchemaValidator:
    def __init__(self):
        self.schema_directory = os.path.join(os.path.dirname(__file__), "../../../../../../pkg/security/tests/schemas")
        self.schema_store = {}
        for filename in os.listdir(self.schema_directory):
            if filename.endswith('.json'):
                with open(os.path.join(self.schema_directory, filename)) as file:
                    schema = json.load(file)
                    if "$id" in schema:
                        # Add each schema to the store using its 'id' as key
                        self.schema_store[f"/schemas/{schema['$id']}"] = schema

        # Create a resolver that uses the schema store for resolving references
        self.resolver = RefResolver(base_uri='', referrer=None, store=self.schema_store)

    def validate_json_data(self, schema_filename, json_data):
        # Validate the instance using the references
        validator = Draft7Validator(self.schema_store[f"/schemas/{schema_filename}"], resolver=self.resolver)
        validator.validate(json_data)
