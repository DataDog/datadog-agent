NVCC = /usr/local/cuda/bin/nvcc
TEMPL_GENERATOR = ./gen-heavy-sample.go

all: sample heavy-sample

.PRECIOUS: %.cu

%.o: %.cu
	$(NVCC) -ccbin g++ -m64 -Xcompiler -ggdb -Xcompiler -pg --threads 0 --std=c++11 \
	-gencode arch=compute_50,code=sm_50 \
	-gencode arch=compute_52,code=sm_52 \
	-gencode arch=compute_60,code=sm_60 \
	-gencode arch=compute_61,code=sm_61 \
	-gencode arch=compute_70,code=sm_70 \
	-gencode arch=compute_75,code=sm_75 \
	-gencode arch=compute_80,code=sm_80 \
	-gencode arch=compute_86,code=sm_86 \
	-gencode arch=compute_89,code=sm_89 \
	-gencode arch=compute_90,code=sm_90 \
	-gencode arch=compute_90,code=compute_90 \
	-o $@ -c $<

%: %.o
	$(NVCC) -ccbin g++ -m64 -Xcompiler -ggdb -Xcompiler -pg \
	-gencode arch=compute_50,code=sm_50 \
	-gencode arch=compute_52,code=sm_52 \
	-gencode arch=compute_60,code=sm_60 \
	-gencode arch=compute_61,code=sm_61 \
	-gencode arch=compute_70,code=sm_70 \
	-gencode arch=compute_75,code=sm_75 \
	-gencode arch=compute_80,code=sm_80 \
	-gencode arch=compute_86,code=sm_86 \
	-gencode arch=compute_89,code=sm_89 \
	-gencode arch=compute_90,code=sm_90 \
	-gencode arch=compute_90,code=compute_90 \
	-o $@ $<

%.cu: %.cu.tmpl $(TEMPL_GENERATOR)
	go run $(TEMPL_GENERATOR) -template $< -output $@ -kernels 80 -variables 10 -shared-memory 1024
