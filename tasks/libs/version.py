from copy import deepcopy


class Version:
    def __init__(self, major, minor, patch=None, rc=None, prefix=""):
        self.prefix = prefix
        self.major = major
        self.minor = minor
        self.patch = patch
        self.rc = rc

    def _safe_value(self, part):
        # Transform None values into 0, as for comparison purposes a None
        # field is equivalent to 0.
        return getattr(self, part) if getattr(self, part) is not None else 0

    def __str__(self):
        version = "{}{}.{}".format(self.prefix, self.major, self.minor)
        if self.patch is not None:
            version = "{}.{}".format(version, self.patch)
        if self.rc is not None and self.rc != 0:
            version = "{}-rc.{}".format(version, self.rc)
        return version

    def __eq__(self, other):
        if not other:
            return False

        if not isinstance(other, Version):
            raise TypeError("Cannot compare Version object with {}".format(type(other)))

        res = True
        # If one value is None, it is equivalent to 0
        for part in ["prefix", "major", "minor", "patch", "rc"]:
            res = res and (self._safe_value(part) == other._safe_value(part))

        return res

    def __gt__(self, other):
        if not other:
            return True

        if not isinstance(other, Version):
            raise TypeError("Cannot compare Version object with {}".format(type(other)))

        for part in ["major", "minor", "patch"]:
            self_part = self._safe_value(part)
            other_part = other._safe_value(part)

            if self_part != other_part:
                return self_part > other_part

        if self.rc is None or other.rc is None:
            # Everything else being equal, self can only be higher than other if other is not a released version
            return other.rc is not None

        return self.rc > other.rc

    def is_rc(self):
        return self._safe_value("rc") != 0

    def next_version(self, bump_major=False, bump_minor=False, bump_patch=False, rc=False):
        new_version = deepcopy(self)

        if bump_patch:
            new_version.patch = self._safe_value("patch") + 1
        elif bump_minor:
            new_version.minor = self._safe_value("minor") + 1
            new_version.patch = 0
        elif bump_major:
            new_version.major = self._safe_value("major") + 1
            new_version.minor = 0
            new_version.patch = 0

        if rc:
            # Bump the rc version
            new_version.rc = new_version._safe_value("rc") + 1
        else:
            # Promote the current version to a non-rc version
            new_version.rc = 0

        return new_version
