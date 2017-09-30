# Build stage
FROM golang:1.9 as build
WORKDIR /go/src/github.com/mattbostock/timbala
RUN apt-get update
RUN apt-get upgrade -y ca-certificates
RUN apt-get install -y git make
COPY . /go/src/github.com/mattbostock/timbala
RUN make

# Main stage
FROM scratch
EXPOSE 9080
LABEL maintainer="matt@mattbostock.com"
COPY --from=build /etc/ssl/certs /etc/ssl/certs
COPY --from=build /go/bin/timbala /
ENTRYPOINT ["/timbala"]
