FROM golang:1.26-alpine AS builder
WORKDIR /

# Install git and certificates
RUN apk --no-cache add tzdata zip ca-certificates git make
COPY go.* ./
RUN go mod download
COPY . ./
RUN --mount=type=cache,target=/root/.cache/go-build env GOOS=linux GOARCH=amd64 go build -v -tags=goexperiment.jsonv2 -o out/ -ldflags="-s -w -X github.com/coredns/coredns/coremain.GitCommit=$(git describe --dirty --always)" .

FROM scratch
WORKDIR /app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/nfd-coredns /app/coredns

EXPOSE 53 53/udp
ENTRYPOINT ["/app/coredns"]
