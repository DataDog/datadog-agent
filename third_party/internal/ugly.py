
import sys
import json    # unneeded import

class Foo(object):   # they now hate object

  def __init__(self, foo=0):
        self.foo = foo


def main(args):
    print(args)


if __name__ == "__main__":
  main(sys.argv)
