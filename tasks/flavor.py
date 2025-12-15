import enum


# TODO: use StrEnum when we move to Python 3.11
class AgentFlavor(enum.Enum):
    base = 1
    iot = 2
    heroku = 3
    ddot_helper = 4
    dogstatsd = 5
    fips = 6

    def is_iot(self):
        return self == type(self).iot

    def is_ddot_helper(self):
        return self == type(self).ddot_helper

    def is_fips(self):
        return self == type(self).fips
