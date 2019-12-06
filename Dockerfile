FROM ubuntu:16.04

SHELL ["/bin/bash", "-c"]

# Install dependencies from apt and then tidy up cache
RUN apt-get update && \
    apt-get install -y vim nano curl wget git build-essential

# Install Go
RUN curl -L -o /tmp/go.tar.gz https://dl.google.com/go/go1.11.4.linux-amd64.tar.gz && \
    tar -xzf /tmp/go.tar.gz -C /usr/local && \
    rm /tmp/go.tar.gz
ENV PATH=${PATH}:/usr/local/go/bin:/root/go/bin

# Install a few Go dependencies
RUN go get -u github.com/derekparker/delve/cmd/dlv

RUN apt-get install -y nginx
