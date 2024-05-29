def color_message(message, color):
    colors = {
        "magenta": "\033[95m",
        "blue": "\033[94m",
        "green": "\033[92m",
        "orange": "\033[93m",
        "red": "\033[91m",
        "grey": "\033[37m",
        "bold": "\33[1m",
    }
    endc = "\033[0m"
    return f"{colors[color]}{message}{endc}" if color in colors else message
