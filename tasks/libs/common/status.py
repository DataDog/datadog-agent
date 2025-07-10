from enum import Enum

from tasks.libs.common.color import Color, color_message


class Status(Enum):
    OK = "OK"
    WARN = "WARN"
    FAIL = "FAIL"

    @staticmethod
    def color(status):
        if status == Status.OK:
            return Color.GREEN

        return Color.ORANGE if status == Status.WARN else Color.RED

    def __str__(self):
        return color_message(self.name, Status.color(self))
