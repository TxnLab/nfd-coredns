FROM golang:1.25-alpine AS builder
WORKDIR /

# Install git and certificates
RUN apk --no-cache add tzdata ca-certificates git
COPY go.* ./
RUN go mod download
COPY . ./
RUN --mount=type=cache,target=/root/.cache/go-build env GOOS=linux GOARCH=amd64 go build -v -o out/did-resolver -ldflags="-s -w" ./did/cmd/did-resolver

FROM scratch
WORKDIR /app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/did-resolver /app/did-resolver

EXPOSE 8080
ENTRYPOINT ["/app/did-resolver"]
