import os


def diff(dir1: str, dir2: str):
    from binary import convert_units

    seen = set()
    dir1not2 = []

    for dirpath, _, filenames in os.walk(dir1):
        for filename in filenames:
            dir1filepath = os.path.join(dirpath, filename)
            relfilepath = dir1filepath.removeprefix(dir1)
            dir2filepath = dir2 + relfilepath

            if not os.path.exists(dir2filepath) and not os.path.islink(dir2filepath):
                dir1not2.append(relfilepath)
                continue

            seen.add(relfilepath)
            s1 = os.stat(dir1filepath, follow_symlinks=False)
            s2 = os.stat(dir2filepath, follow_symlinks=False)
            if s1.st_size != s2.st_size:
                diff = s2.st_size - s1.st_size
                sign = "+" if diff > 0 else "-"
                amount, unit = convert_units(abs(diff))
                amount = round(amount, 2)
                print(f"Size mismatch: {relfilepath} {s1.st_size} vs {s2.st_size} ({sign}{amount}{unit})")

    print()

    if dir1not2:
        print(f"Files in {dir1} but not in {dir2}:")
        for filepath in dir1not2:
            print(filepath)

        print()

    header = False
    for dirpath, _, filenames in os.walk(dir2):
        for filename in filenames:
            dir2filepath = os.path.join(dirpath, filename)
            relfilepath = dir2filepath.removeprefix(dir2)
            if relfilepath not in seen:
                if not header:
                    print(f"Files in {dir2} but not in {dir1}:")
                    header = True
                print(relfilepath)
