FROM golang:1.9
LABEL maintainer="matt@mattbostock.com"

WORKDIR /go/src/github.com/mattbostock/athensdb
COPY . /go/src/github.com/mattbostock/athensdb

RUN go test -c -o /bin/athensdb.test -race -tags integration ./internal/test/integration

ENTRYPOINT ["/bin/athensdb.test"]
