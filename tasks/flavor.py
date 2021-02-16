from email.mime import base
import enum


# TODO: use StrEnum when we move to Python 3.11
class AgentFlavor(enum.Enum):
    base = 1
    iot = 2
    heroku = 3
    dogstatsd = 4
    core = 5

    def is_iot(self):
        return self == type(self).iot

    def to_cmd_path(self):
        if self.is_iot():
            return "iot-agent"

        if self == type(self).core:
            return "agent"

        return "meta"
