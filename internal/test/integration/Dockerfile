FROM golang:1.8
MAINTAINER Matt Bostock <matt@mattbostock.com>

WORKDIR /go/src/github.com/mattbostock/athensdb
COPY . /go/src/github.com/mattbostock/athensdb

RUN go test -c -o /bin/athensdb.test -race -tags integration ./internal/test/integration && \
  cd /bin && \
  rm -rf /go/src

ENTRYPOINT ["athensdb.test"]
