from invoke.collection import Collection

from tasks.schema.generate import generate

collection = Collection()
collection.add_task(generate)
