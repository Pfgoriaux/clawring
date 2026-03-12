FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /clawring .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates \
    && mkdir -p /etc/openclaw-proxy /var/lib/openclaw-proxy
COPY --from=builder /clawring /usr/local/bin/clawring
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
EXPOSE 9100 9101
ENTRYPOINT ["/entrypoint.sh"]
