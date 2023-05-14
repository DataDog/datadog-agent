import emoji


class Step:
    def __init__(self, msg="", emoji=""):
        self.msg = msg
        self.emoji = emoji

    def __enter__(self):
        _emoji = emoji.emojize(self.emoji)
        print(f"{_emoji} {self.msg}... ", end="", flush=True)
        return self

    def __exit__(self, exc_type, _exc_val, _exc_tb):
        if exc_type is None:
            print(emoji.emojize(":check_mark:"), flush=True)
        else:
            print(emoji.emojize(":cross_mark:"), flush=True)
