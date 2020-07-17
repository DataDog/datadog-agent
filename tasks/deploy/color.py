def color_str(str, color):
    colors = {
        "blue": "\033[94m",
        "green": "\033[92m",
        "orange": "\033[93m",
        "red": "\033[91m",
        "grey": "\033[37m",
        "bold": "\33[1m",
    }
    endc = "\033[0m"
    return u"{}{}{}".format(colors[color], str, endc) if color in colors else str
