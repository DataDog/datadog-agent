import json
import os
from jsonschema import Draft7Validator

class JsonSchemaValidator:
    def __init__(self, schema_directory):
        self.schema_directory = schema_directory
        self.schema_cache = {}

    def _read_schema_file(self, schema_filename):
        schema_filepath = os.path.join(self.schema_directory, schema_filename)
        with open(schema_filepath, 'r') as f:
            schema = json.load(f)
        return schema

    def _load_and_cache_schema(self, schema_filename):
        schema = self._read_schema_file(schema_filename)
        self.schema_cache[schema_filename] = Draft7Validator(schema)
        return self.schema_cache[schema_filename]

    def get_schema_validator(self, schema_filename):
        if schema_filename not in self.schema_cache:
            return self._load_and_cache_schema(schema_filename)
        return self.schema_cache[schema_filename]

    def validate_json_data(self, schema_filename, json_data):
        validator = self.get_schema_validator(schema_filename)
        try:
            validator.validate(json_data)
            return True, None
        except Exception as e:
            return False, str(e)