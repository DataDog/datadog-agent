class Status:
    OK = "OK"
    WARN = "WARN"
    FAIL = "FAIL"

    @staticmethod
    def color(status):
        if status == Status.OK:
            return "green"

        return "orange" if status == Status.WARN else "red"
