import os

from dotenv import load_dotenv
from trello import TrelloClient


class LocalBoard:
    """A LocalBoard is like a Trello Board, but just prints cards to the console."""

    def add_list(self, name):
        return LocalList(name)


class LocalList:
    """A LocalList is like a Trello List, but just prints cards to the console."""

    def __init__(self, name):
        self.name = name

    def add_card(self, name, content):
        heading = f"{name} [List: {self.name}]"
        print("=" * len(heading))
        print(heading)
        print("=" * len(heading))
        print("")
        print(content)
        print("")


def setup(version):
    # Setup env
    load_dotenv()
    api_key = os.getenv('API_KEY')
    api_secret = os.getenv('API_SECRET')
    org_id = os.getenv('ORG_ID')

    if api_key and api_secret and org_id:
        # Setup trello
        client = TrelloClient(api_key=api_key, api_secret=api_secret)
        board = client.add_board("[" + version + "] Logs Agent Release QA", None, org_id)

        for list in board.all_lists():
            list.close()
        board.add_list("Done")
        return board

    return LocalBoard()


def finish(board):
    if not isinstance(board, LocalBoard):
        print("Your QA board is ready: ")
        print(board.url)
