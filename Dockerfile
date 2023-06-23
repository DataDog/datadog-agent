FROM ubuntu

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt install -yq  software-properties-common
RUN add-apt-repository ppa:deadsnakes/ppa
RUN apt-get install -yq python3.9 python3-pip python3.9-dev python3-venv python3.9-distutils python3-psutil
RUN apt-get install -yq git build-essential liblz4-dev libunwind-dev

RUN pip install pipx
RUN which pipx
ENV PATH=/root/.local/bin:$PATH
RUN pipx install ruff
RUN pipx install ddev