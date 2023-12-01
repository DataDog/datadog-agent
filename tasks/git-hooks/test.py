import re
import subprocess


def get_resolved_dependencies():
    requirements_file_path = 'requirements.txt'

    try:
        result = subprocess.run(
            ['pip', 'install', '-r', requirements_file_path, '--report', '--dry-run', '--ignore-installed'],
            capture_output=True,
            text=True,
            check=True,
        )
        print(result.stdout)
        # Extract resolved dependencies from the "Would install" line
        match = re.search(r'Would install\s*(.*)', result.stdout, re.DOTALL)
        print(match)
        if match:
            resolved_dependencies = [pkg.strip() for pkg in match.group(1).split(' ')]
            return resolved_dependencies
        else:
            return []

    except subprocess.CalledProcessError as e:
        print("Error:", e.stderr)
        raise SystemExit("Failed to get resolved dependencies.")


if __name__ == "__main__":
    resolved_dependencies = get_resolved_dependencies()

    if resolved_dependencies:
        print("Resolved dependencies:")
        for dep in resolved_dependencies:
            print(dep)
    else:
        print("No dependencies resolved.")
