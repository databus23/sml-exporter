FROM golang:alpine

WORKDIR /build
ADD go.* *.go .
RUN go build -o sml-exporter

FROM alpine
COPY --from=0 /build/sml-exporter /usr/local/bin/
