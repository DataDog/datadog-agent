from invoke.collection import Collection

from tasks.schema.generate import compress, generate
from tasks.schema.lint import lint as lint_task
from tasks.schema.template import template, template_all

collection = Collection()
collection.add_task(generate)
collection.add_task(lint_task)
collection.add_task(compress)
collection.add_task(template)
collection.add_task(template_all)
