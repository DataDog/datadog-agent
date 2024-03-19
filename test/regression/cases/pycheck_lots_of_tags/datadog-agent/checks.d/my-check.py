import random
import string

from datadog_checks.checks import AgentCheck

def generate_tag_sets(rng, num_sets, tags_per_set, tag_length, unique_tagset_ratio):
    """
    Generate tag sets with a specified ratio, at the tagset level, of unique strings to potentially reused tag sets,
    using a specified seed for reproducibility.

    Parameters:
    - rng (Random): pre-seeded entropy source
    - num_sets (int): Number of tag sets to generate.
    - tags_per_set (int): Number of tags in each set.
    - tag_length (int): Total length of each tag, including the delimiter.
    - unique_tag_ratio (float): Value between 1 and 0, indicating the ratio of unique tags.
                                If this value is 1, every tag will be unique. If 0, all will be re-used.
    - seed (int): Seed value for random number generator to ensure reproducibility.

    Returns:
    - List[List[str]]: A list of tag sets.
    """

    individual_tags = []
    def generate_tag(tag_length):
        if rng.random() >= unique_tagset_ratio and len(individual_tags) != 0:
            # sample from existing tags
            return rng.choice(individual_tags)
        # Else, generate a new unique tag

        if tag_length % 2 == 0:
            half_length = tag_length // 2 - 1
        else:
            half_length = (tag_length - 1) // 2

        if tag_length % 2 == 0 and rng.choice([True, False]):
            left_length = half_length + 1
            right_length = half_length
        else:
            left_length = half_length
            right_length = half_length + 1 if tag_length % 2 == 0 else half_length

        left_part = ''.join(rng.choice(string.ascii_letters + string.digits) for _ in range(left_length))
        right_part = ''.join(rng.choice(string.ascii_letters + string.digits) for _ in range(right_length))
        tag = f"{left_part}:{right_part}"
        individual_tags.append(tag)
        return tag

    tag_sets = []

    for _ in range(num_sets):
        current_set = set()
        while len(current_set) < tags_per_set:
            current_set.add(generate_tag(tag_length))
        tag_sets.append(list(current_set))

    return tag_sets



class MyCheck(AgentCheck):
    def check(self, instance):
        seed = instance.get("seed", 11235813)
        rng = random.Random()
        rng.seed(seed)

        num_tagsets = instance.get("num_tagsets", 10)
        tags_per_set = instance.get("tags_per_set", 10)
        tag_length = instance.get("tag_length", 100)
        unique_tagset_ratio = instance.get("unique_tagset_ratio", 0.11)
        num_metrics = instance.get("num_metrics", 100)
        tag_sets = generate_tag_sets(rng, num_tagsets, tags_per_set, tag_length, unique_tagset_ratio)

        for _ in range(num_metrics):
            # For each metric that gets submitted, choose a tagset at random
            # This will average out to
            # contexts = len(tag_sets) as long as num_metrics is greater than num_tagsets
            self.gauge('hello.world', rng.random() * 1000, tags=rng.choice(tag_sets))
