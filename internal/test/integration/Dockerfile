FROM golang:1.9
LABEL maintainer="matt@mattbostock.com"

WORKDIR /go/src/github.com/mattbostock/timbala
COPY . /go/src/github.com/mattbostock/timbala

RUN go test -c -o /bin/timbala.test -race -tags integration ./internal/test/integration

ENTRYPOINT ["/bin/timbala.test"]
