FROM centos:7

ARG GOLANG_VERSION=1.10.3
ARG GIMME_URL=raw.githubusercontent.com/travis-ci/gimme/master/gimme
ARG GIMME_DEBUG=0
ARG RBENV_VERSION=2.4.4
ARG BUNDLER_VERSION=1.15.3

RUN yum -y install git bzip2 gcc make openssl-devel readline-devel zlib-devel \
                   python-devel.x86_64 patch gcc-c++ file rpm-build

RUN yum install -y python  \
    && curl "https://bootstrap.pypa.io/get-pip.py" -o "get-pip.py" \
    && python get-pip.py \
    && rm get-pip.py \
    && pip install virtualenv==16.0.0

RUN eval "$(curl -sL https://$GIMME_URL | \
    GIMME_GO_VERSION="$GOLANG_VERSION" \
    GIMME_DEBUG=$GIMME_DEBUG \
    bash)"

ENV GOPATH /go
ENV GOBIN /root/.gimme/versions/go"$GOLANG_VERSION".linux.amd64/bin
ENV PATH="$HOME"/bin:/root/.gimme/versions/go"$GOLANG_VERSION".linux.amd64/bin:/usr/local/go/bin:/go/src/github.com/bin:$PATH

RUN mkdir -p $GOPATH $HOME/bin/ /usr/local/go/bin \
    && echo $PATH && go version

WORKDIR $GOPATH

RUN curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh

RUN go get -u golang.org/x/lint/golint

RUN git clone git://github.com/rbenv/rbenv.git /usr/local/rbenv \
    &&  git clone git://github.com/rbenv/ruby-build.git /usr/local/rbenv/plugins/ruby-build \
    &&  git clone git://github.com/jf/rbenv-gemset.git /usr/local/rbenv/plugins/rbenv-gemset \
    &&  /usr/local/rbenv/plugins/ruby-build/install.sh

ENV PATH /usr/local/rbenv/bin:$PATH
ENV RBENV_ROOT /usr/local/rbenv

RUN echo 'export RBENV_ROOT=/usr/local/rbenv' >> /etc/profile.d/rbenv.sh \
    &&  echo 'export PATH=/usr/local/rbenv/bin:$PATH' >> /etc/profile.d/rbenv.sh \
    &&  echo 'eval "$(rbenv init -)"' >> /etc/profile.d/rbenv.sh

RUN echo 'export RBENV_ROOT=/usr/local/rbenv' >> /root/.bashrc \
    &&  echo 'export PATH=/usr/local/rbenv/bin:$PATH' >> /root/.bashrc \
    &&  echo 'eval "$(rbenv init -)"' >> /root/.bashrc

ENV CONFIGURE_OPTS --disable-install-doc
ENV PATH /usr/local/rbenv/bin:/usr/local/rbenv/shims:$PATH

RUN eval "$(rbenv init -)"; rbenv install $RBENV_VERSION \
    &&  eval "$(rbenv init -)"; rbenv global $RBENV_VERSION \
    &&  eval "$(rbenv init -)"; gem update --system \
    &&  eval "$(rbenv init -)"; gem install bundler -f --version "$BUNDLER_VERSION" \
    &&  rm -rf /tmp/*

ENV GEM_HOME /usr/local/bundle
ENV BUNDLE_PATH="$GEM_HOME" \
    BUNDLE_BIN="$GEM_HOME/bin" \
    BUNDLE_SILENCE_ROOT_WARNING=1 \
    BUNDLE_APP_CONFIG="$GEM_HOME"
ENV PATH $BUNDLE_BIN:$PATH
RUN mkdir -p "$GEM_HOME" "$BUNDLE_BIN" \
    && chmod 777 "$GEM_HOME" "$BUNDLE_BIN"

RUN git config --global user.email "gitlab@stackstate" && git config --global user.name "Gitlab Stackstate"
