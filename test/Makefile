LIB_PATH = ../../six/:../../two/:../../three/
TWO_PKGS = \
	./two_extend/...

THREE_PKGS = \
	./three_extend/...

PKGS = \
	./init/... \
	./six/...

all: test

test:
	@echo Run Python2 testsuite...
	@DYLD_LIBRARY_PATH=$(LIB_PATH) LD_LIBRARY_PATH=$(LIB_PATH) TEST_TWO= go test -count=1 $(TWO_PKGS) $(PKGS)
	@echo
	@echo Run Python3 testsuite...
	@DYLD_LIBRARY_PATH=$(LIB_PATH) LD_LIBRARY_PATH=$(LIB_PATH) go test -count=1 $(THREE_PKGS) $(PKGS)
