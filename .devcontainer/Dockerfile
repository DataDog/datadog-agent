FROM golang:latest

# Configure to avoid build warnings and errors as described in official VSCode Remote-Containers extension documentation.
# See https://code.visualstudio.com/docs/remote/containers-advanced#_reducing-dockerfile-build-warnings.
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
    && apt-get -y install --no-install-recommends apt-utils 2>&1

# Verify git, process tools, lsb-release (common in install instructions for CLIs) installed.
RUN apt-get -y install git iproute2 procps lsb-release
# Install Python2.7
RUN apt-get install -y python2.7 python-pip

# Install essential tools for Go development.
RUN apt-get update \
    # Install https://github.com/stamblerre/gocode as gocode-gomod (required by VSCode Go extension).
    && go get -x -d github.com/stamblerre/gocode 2>&1 \
    && go build -o gocode-gomod github.com/stamblerre/gocode \
    && mv gocode-gomod $GOPATH/bin/ \
    # Install other essential Go packages and tools.
    && go get -u -v \
    github.com/golang/dep/cmd/dep \
    golang.org/x/tools/gopls \
    github.com/mdempsky/gocode \
    github.com/uudashr/gopkgs/cmd/gopkgs \
    github.com/ramya-rao-a/go-outline \
    github.com/acroca/go-symbols \
    golang.org/x/tools/cmd/guru \
    golang.org/x/tools/cmd/gorename \
    github.com/go-delve/delve/cmd/dlv \
    github.com/stamblerre/gocode \
    github.com/rogpeppe/godef \
    golang.org/x/tools/cmd/goimports \
    golang.org/x/lint/golint 2>&1 \
    # Clean up.
    && apt-get autoremove -y \
    && apt-get clean -y \
    && rm -rf /var/lib/apt/lists/*

# Revert configurations that was set at top layer (for avoiding build warnings and errors).
ENV DEBIAN_FRONTEND=dialog

# Expose service ports.
EXPOSE 8000
