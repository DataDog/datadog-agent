import emoji


class Step:
    def __init__(self, msg="", emoji=""):
        self.msg = msg
        self.emoji = emoji

    def __enter__(self):
        print("{} {}... ".format(emoji.emojize(self.emoji), self.msg), end="", flush=True)
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        print(emoji.emojize(":check_mark:"))
