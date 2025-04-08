FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 go build -o accel-exporter ./cmd/accel-exporter

FROM alpine:latest

RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/accel-exporter .

EXPOSE 9101
ENTRYPOINT ["./accel-exporter"]
