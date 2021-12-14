import enum


# TODO: use StrEnum when we move to Python 3.11
class AgentFlavor(enum.Enum):
    base = 1
    iot = 2

    def equals(self, arg):
        return arg == self.name

    @classmethod
    def is_iot(cls, arg):
        return cls.iot.equals(arg)
