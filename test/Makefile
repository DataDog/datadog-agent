LIB_PATH = ../../six/:../../two/:../../three/
TWO_PKGS = \
	./two/... \
	./two_extend/... \
	./two_init/... \
	./two_check

THREE_PKGS = \
	./three/... \
	./three_extend/... \
	./three_init/... \
	./three_check

all: test

test:
	DYLD_LIBRARY_PATH=$(LIB_PATH) LD_LIBRARY_PATH=$(LIB_PATH) go test $(TWO_PKGS)
	@echo
	DYLD_LIBRARY_PATH=$(LIB_PATH) LD_LIBRARY_PATH=$(LIB_PATH) go test $(THREE_PKGS)
