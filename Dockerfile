FROM golang:1.8-alpine
MAINTAINER Matt Bostock <matt@mattbostock.com>

EXPOSE 9080

WORKDIR /go/src/github.com/mattbostock/athensdb
COPY . /go/src/github.com/mattbostock/athensdb

RUN apk add --no-cache git make && \
  make && \
  apk del make && \
  cd && \
  rm -rf /go/src

ENTRYPOINT ["/go/bin/athensdb"]
