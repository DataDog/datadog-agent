import os


def diff(dir1: str, dir2: str, sort_by_size: bool = False):
    from binary import convert_units

    seen = set()
    dir1not2 = []  # Files in dir1 but not in dir2 (removed files)
    dir2not1 = []  # Files in dir2 but not in dir1 (new files)
    size_mismatches = []  # Files with size differences

    # First pass: walk dir1 and collect differences
    for dirpath, _, filenames in os.walk(dir1):
        for filename in filenames:
            dir1filepath = os.path.join(dirpath, filename)
            relfilepath = dir1filepath.removeprefix(dir1)
            dir2filepath = dir2 + relfilepath

            if not os.path.exists(dir2filepath) and not os.path.islink(dir2filepath):
                s1 = os.stat(dir1filepath, follow_symlinks=False)
                dir1not2.append((relfilepath, s1.st_size))
                continue

            seen.add(relfilepath)
            s1 = os.stat(dir1filepath, follow_symlinks=False)
            s2 = os.stat(dir2filepath, follow_symlinks=False)
            if s1.st_size != s2.st_size:
                size_mismatches.append((relfilepath, s1.st_size, s2.st_size))

    # Second pass: walk dir2 to find new files
    for dirpath, _, filenames in os.walk(dir2):
        for filename in filenames:
            dir2filepath = os.path.join(dirpath, filename)
            relfilepath = dir2filepath.removeprefix(dir2)
            if relfilepath not in seen:
                s2 = os.stat(dir2filepath, follow_symlinks=False)
                dir2not1.append((relfilepath, s2.st_size))

    # Sort if requested
    if sort_by_size:
        # Sort size mismatches by decreasing absolute change
        size_mismatches.sort(key=lambda x: x[2] - x[1], reverse=True)
        # Sort removed and new files by decreasing size
        dir1not2.sort(key=lambda x: x[1], reverse=True)
        dir2not1.sort(key=lambda x: x[1], reverse=True)

    # Print size mismatches
    for relfilepath, s1_size, s2_size in size_mismatches:
        diff_bytes = s2_size - s1_size
        sign = "+" if diff_bytes > 0 else "-"
        amount, unit = convert_units(abs(diff_bytes))
        amount = round(amount, 2)
        print(f"Size mismatch: {relfilepath} {s1_size} vs {s2_size} ({sign}{amount}{unit})")

    print()

    # Print removed files
    if dir1not2:
        print(f"Files in {dir1} but not in {dir2}:")
        for filepath, size in dir1not2:
            amount, unit = convert_units(size)
            amount = round(amount, 2)
            print(f"{filepath} (-{amount}{unit})")

        print()

    # Print new files
    if dir2not1:
        print(f"Files in {dir2} but not in {dir1}:")
        for filepath, size in dir2not1:
            amount, unit = convert_units(size)
            amount = round(amount, 2)
            print(f"{filepath} (+{amount}{unit})")
