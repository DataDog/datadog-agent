LIB_PATH = ../../six/:../../two/:../../three/
THREE_PKGS = \
	./three_extend/...
PKGS = \
	./init/... \
	./six/... \
	./aggregator/...

all: test

test:
	# due to the nature of the tests, disable caching and parallelism
	@echo Run Python2 testsuite...
	@DYLD_LIBRARY_PATH=$(LIB_PATH) LD_LIBRARY_PATH=$(LIB_PATH) TEST_TWO= go test -count=1 -p=1 $(PKGS)
	@echo
	@echo Run Python3 testsuite...
	@DYLD_LIBRARY_PATH=$(LIB_PATH) LD_LIBRARY_PATH=$(LIB_PATH) go test -count=1 -p=1 $(THREE_PKGS) $(PKGS)
