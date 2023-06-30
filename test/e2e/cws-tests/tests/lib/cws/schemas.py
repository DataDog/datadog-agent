import json
import os
from jsonschema import RefResolver, Draft7Validator, validators
from datetime import datetime


class JsonSchemaValidator:
    def __init__(self):
        self.schema_directory = os.path.join(os.path.dirname(__file__), "schemas")
        self.schema_store = {}
        for filename in os.listdir(self.schema_directory):
            if filename.endswith('.json'):
                with open(os.path.join(self.schema_directory, filename)) as file:
                    schema = json.load(file)
                    # Add each schema to the store using its 'id' as key
                    self.schema_store[schema['$id']] = schema

        # Create a resolver that uses the schema store for resolving references
        self.resolver = RefResolver(base_uri='', referrer=None, store=self.schema_store)

    def is_datetime(self, inst):
        return isinstance(inst, datetime)

    def validate_json_data(self, schema_filename, json_data):
        date_check = Draft7Validator.TYPE_CHECKER.redefine('datetime', self.is_datetime)
        CustomValidator = validators.extend(Draft7Validator, type_checker=date_check)
        validator = CustomValidator(self.schema_store[schema_filename], resolver=self.resolver)

        # Validate the instance
        validator.validate(json_data)
