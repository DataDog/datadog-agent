This folder contains sample data to test the fatbin parser. The sample.cu file can be compiled just with running `make`.

The compiler required is `nvcc`, to install in Ubuntu run `sudo apt-get install nvidia-cuda-dev`. Use `cuobjdump` (from the same package) to dump the fatbin file manually and check the results. The argument `-res-usage` can be used in that command to check the resource usage for each kernel (note that sometimes, the resource usage for the same kernel might vary between architectures).

The sample binary is checked into source for tests, to avoid having the dependency on the compiler in the CI.
