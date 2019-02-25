LIB_PATH = ../../six/:../../two/:../../three/
TWO_PKGS = \
	./two/... \
	./two_extend/... \
	./two_init/...

THREE_PKGS = \
	./three/... \
	./three_extend/... \
	./three_init/... \

PKGS = \
	./aggregator/...

all: test

test:
	@DYLD_LIBRARY_PATH=$(LIB_PATH) LD_LIBRARY_PATH=$(LIB_PATH) TEST_TWO= go test $(TWO_PKGS) $(PKGS)
	@echo
	@DYLD_LIBRARY_PATH=$(LIB_PATH) LD_LIBRARY_PATH=$(LIB_PATH) go test $(THREE_PKGS) $(PKGS)
