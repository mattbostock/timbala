# Build stage
FROM golang:1.8 as build
WORKDIR /go/src/github.com/mattbostock/athensdb
RUN apt-get update
RUN apt-get install -y git make
COPY . /go/src/github.com/mattbostock/athensdb
RUN make

# Main stage
FROM alpine:latest  
EXPOSE 9080
LABEL maintainer="matt@mattbostock.com"
RUN apk --no-cache add ca-certificates
COPY --from=build /go/bin/athensdb /
ENTRYPOINT ["/athensdb"]
