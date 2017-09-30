FROM golang:1.9
LABEL maintainer="matt@mattbostock.com"

WORKDIR /go/src/github.com/mattbostock/timbala
COPY . /go/src/github.com/mattbostock/timbala

RUN go build -o /bin/timbala.bench -tags bench ./internal/test/bench

ENTRYPOINT ["/bin/timbala.bench"]
