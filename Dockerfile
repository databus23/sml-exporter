FROM golang:alpine

RUN apk add --no-cache git make gcc musl-dev
WORKDIR /build
RUN git clone https://github.com/volkszaehler/libsml \
      && sed -i'' /uuid/d libsml/sml/Makefile \
      && sed -i'' /uuid/d libsml/examples/Makefile
RUN CFLAGS=-DSML_NO_UUID_LIB make -C libsml/sml
RUN make -C libsml/examples

ADD go.* *.go .
RUN go build -o sml-exporter

FROM alpine
COPY --from=0 /build/sml-exporter /build/libsml/examples/sml_server /usr/local/bin/
RUN sml_server -h
