import subprocess


# This function cannot be defined in a file that imports invoke.tasks. Otherwise it fails when called with multiprocessing.
def destroy_remote_stack(stack: str):
    return subprocess.run(
        ["pulumi", "destroy", "--remove", "--yes", "--stack", stack], capture_output=True, text=True
    ), stack
