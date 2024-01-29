This code was taken from https://github.com/kubernetes/client-go/tree/v0.27.0/tools/leaderelection in its entirety as v0.28.0 and higher 
completely removed configmaplock, EndpointsLeases and ConfigMapsLeases as part of https://github.com/kubernetes/client-go/commit/27bbe765353fa01ed516b11bde1da82ae6f6c3bc

We need to maintain compatibility with k8s <1.14 and as such need to still use ConfigMapLeases in those older versions.
