FROM golang:1.8-alpine
MAINTAINER Matt Bostock <matt@mattbostock.com>

WORKDIR /go/src/github.com/mattbostock/athensdb
COPY . /go/src/github.com/mattbostock/athensdb

RUN go test -c -o /bin/athensdb.test -tags integration ./integration_tests && \
  cd /bin && \
  rm -rf /go/src

ENTRYPOINT ["athensdb.test"]
