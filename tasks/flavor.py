import enum


# TODO: use StrEnum when we move to Python 3.11
class AgentFlavor(enum.Enum):
    base = 1
    iot = 2
    heroku = 3
    dogstatsd = 4
    fips = 5
    recorder = 6

    def is_iot(self):
        return self == type(self).iot

    def is_fips(self):
        return self == type(self).fips

    def fips_backend(self):
        return FipsBackend.msgo if self.is_fips() else FipsBackend.none


class FipsBackend(enum.StrEnum):
    none = enum.auto()
    msgo = enum.auto()  # Microsoft Go's systemcrypto: OpenSSL on Linux, CNG on Windows
    boringcrypto = enum.auto()  # upstream Go's BoringSSL backend

    def agent_flavor(self):
        return AgentFlavor.base if self == FipsBackend.none else AgentFlavor.fips
