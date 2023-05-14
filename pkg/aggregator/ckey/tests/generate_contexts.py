import random
import uuid

f = open("random_contexts.csv", "w")
for _ in range(10000000):
    uid = uuid.uuid4()
    uid2 = uuid.uuid4()
    f.write(str(uuid.uuid4()) + ",")

    tags_count = random.randint(1, 4)
    for i in range(tags_count):
        f.write(str(uuid.uuid4()))
        if i < tags_count - 1:
            f.write(" ")
    f.write("\n")
f.close()
