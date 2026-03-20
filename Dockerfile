FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/cdn-scheduler ./cmd

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/cdn-scheduler /usr/local/bin/cdn-scheduler
COPY configs/scheduler-config.yaml /etc/cdn-scheduler/config.yaml
EXPOSE 15353/udp 15353/tcp 8053
CMD ["cdn-scheduler", "-config", "/etc/cdn-scheduler/config.yaml"]
