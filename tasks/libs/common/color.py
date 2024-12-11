# TODO Once we will use it everywhere, switch the value to the color
class Color:
    MAGENTA = "magenta"
    BLUE = "blue"
    GREEN = "green"
    ORANGE = "orange"
    RED = "red"
    GREY = "grey"
    BOLD = "bold"


ENDC = "\33[0m"

# TODO Remove the dict once we moved all the calls to the enum and use the color directly there
COLORS = {
    Color.MAGENTA: "\033[95m",
    Color.BLUE: "\033[94m",
    Color.GREEN: "\033[92m",
    Color.ORANGE: "\033[93m",
    Color.RED: "\033[91m",
    Color.GREY: "\033[37m",
    Color.BOLD: "\33[1m",
}


def color_message(message: str, color: str) -> str:
    return f"{COLORS[color]}{message}{ENDC}" if color in COLORS else message
