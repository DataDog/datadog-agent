from tasks.libs.common.color import Color


class Status:
    OK = "OK"
    WARN = "WARN"
    FAIL = "FAIL"

    @staticmethod
    def color(status):
        if status == Status.OK:
            return Color.GREEN

        return Color.ORANGE if status == Status.WARN else Color.RED
