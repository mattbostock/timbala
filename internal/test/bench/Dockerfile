FROM golang:1.9
LABEL maintainer="matt@mattbostock.com"

WORKDIR /go/src/github.com/mattbostock/athensdb
COPY . /go/src/github.com/mattbostock/athensdb

RUN go build -o /bin/athensdb.bench -tags bench ./internal/test/bench

ENTRYPOINT ["/bin/athensdb.bench"]
