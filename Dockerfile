FROM golang:1.8
LABEL maintainer="matt@mattbostock.com"

EXPOSE 9080

WORKDIR /go/src/github.com/mattbostock/athensdb
COPY . /go/src/github.com/mattbostock/athensdb

RUN apt-get update && \
  apt-get install -y git make && \
  make && \
  apt-get purge -y git make && \
  cd && \
  rm -rf /go/src

ENTRYPOINT ["/go/bin/athensdb"]
