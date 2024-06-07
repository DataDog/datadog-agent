import enum


# TODO: use StrEnum when we move to Python 3.11
class AgentFlavor(enum.Enum):
    base = 1
    iot = 2
    heroku = 3
    dogstatsd = 4

    def is_iot(self):
        return self == type(self).iot
