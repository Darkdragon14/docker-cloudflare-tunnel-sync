FROM golang:1.24-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/docker-cloudflare-tunnel-sync ./cmd/docker-cloudflare-tunnel-sync

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

COPY --from=build /out/docker-cloudflare-tunnel-sync /usr/local/bin/docker-cloudflare-tunnel-sync

ENTRYPOINT ["/usr/local/bin/docker-cloudflare-tunnel-sync"]
