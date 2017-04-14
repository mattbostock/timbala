FROM golang:1.8-alpine
MAINTAINER Matt Bostock <matt@mattbostock.com>

EXPOSE 9080

WORKDIR /go/src/github.com/mattbostock/athens
COPY . /go/src/github.com/mattbostock/athens

RUN apk add --update make && \
  make && \
  apk del make && \
  cd && \
  rm -rf /go/src

ENTRYPOINT ["/go/bin/athens"]
