FROM golang:1.23-alpine AS builder
WORKDIR /build

# Install git and certificates
RUN apk --no-cache add tzdata zip ca-certificates git make

RUN git clone https://github.com/coredns/coredns.git && cd coredns && git checkout v1.12.0
RUN echo "nfd:github.com/TxnLab/nfd-coredns" >> ./plugin.cfg
COPY . /build
COPY docker_goworkfile /build/coredns/go.work
RUN cd coredns && go generate coredns.go && mkdir -p out/linux/amd64 && make coredns BINARY=out/linux/amd64/coredns SYSTEM="GOOS=linux GOARCH=amd64" CHECKS="" BUILDOPTS=""

FROM scratch
WORKDIR /app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /build/coredns/out/linux/amd64/coredns /app/coredns

EXPOSE 53 53/udp
ENTRYPOINT ["/app/coredns"]
