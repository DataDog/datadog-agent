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

HTML_COLORS = {
    "\033[95m":'<span style="color:magenta">',
    "\033[94m":'<span style="color:blue">',
    "\033[92m":'<span style="color:green">',
    "\033[93m":'<span style="color:orange">',
    "\033[91m":'<span style="color:red">',
    "\033[37m":'<span style="color:grey">',
    "\33[1m":'<span style="font-weight:bold">',
    "\33[0m":'</span>'
}

def color_message(message: str, color: str) -> str:
    return f"{COLORS[color]}{message}{ENDC}" if color in COLORS else message

def bash_color_to_html(colored_message: str) -> str:
    for bash_color in HTML_COLORS:
        colored_message = colored_message.replace(bash_color, HTML_COLORS[bash_color])
    return colored_message
