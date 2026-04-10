from invoke.collection import Collection

from tasks.schema.generate import generate
from tasks.schema.template import template, template_all

collection = Collection()
collection.add_task(generate)
collection.add_task(template)
collection.add_task(template_all)
