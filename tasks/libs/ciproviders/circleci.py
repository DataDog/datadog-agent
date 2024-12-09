import re


def update_circleci_config(file_path, image_tag, test):
    """
    Override variables in .gitlab-ci.yml file
    """
    image_name = "gcr.io/datadoghq/agent-circleci-runner"
    with open(file_path) as circle:
        circle_ci = circle.read()
    match = re.search(rf"({image_name}(_test_only)?):([a-zA-Z0-9_-]+)\n", circle_ci)
    if not match:
        raise RuntimeError(f"Impossible to find the version of image {image_name} in circleci configuration file")
    image = f"{image_name}_test_only" if test else image_name
    with open(file_path, "w") as circle:
        circle.write(circle_ci.replace(f"{match.group(0)}", f"{image}:{image_tag}\n"))
