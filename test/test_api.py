from cffi import FFI

def test_foo(lib):
    six = lib.make2()
    print(six)
    assert six
    lib.init(six, FFI.NULL)
