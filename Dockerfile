FROM golang:1.23-alpine AS builder
# Install git and certificates
RUN apk --no-cache add tzdata zip ca-certificates git

FROM scratch
WORKDIR /app

COPY build/coredns/out/linux/amd64/coredns /app/coredns
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

EXPOSE 53 53/udp
ENTRYPOINT ["/app/coredns"]
