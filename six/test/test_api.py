from cffi import FFI


def test_make2(lib):
    six = lib.make2()
    assert six


def test_init2(lib):
    six = lib.make2()
    lib.init(six, FFI.NULL)
    assert lib.is_initialized(six)


def test_make3(lib):
    six = lib.make3()
    assert six


def test_init3(lib):
    six = lib.make3()
    lib.init(six, FFI.NULL)
    assert lib.is_initialized(six)
