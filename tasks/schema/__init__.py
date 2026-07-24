from invoke.collection import Collection

from tasks.schema.add_setting import add_setting
from tasks.schema.check_codegen_drift import check_codegen_drift
from tasks.schema.generate import codegen, compress, generate, produce_embedded, produce_jsonschema
from tasks.schema.lint import lint as lint_task
from tasks.schema.locate import locate as locate_task
from tasks.schema.template import template, template_all

collection = Collection()
collection.add_task(add_setting)
collection.add_task(check_codegen_drift)
collection.add_task(codegen)
collection.add_task(generate)
collection.add_task(lint_task)
collection.add_task(compress)
collection.add_task(template)
collection.add_task(template_all)
collection.add_task(locate_task)
collection.add_task(produce_embedded)
collection.add_task(produce_jsonschema)
