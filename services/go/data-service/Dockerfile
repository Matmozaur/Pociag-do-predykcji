FROM golang:1.25.0-alpine3.22 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /data-service ./cmd

FROM alpine:3.22.1
RUN adduser -D -u 1001 appuser
USER 1001
WORKDIR /app
COPY --from=builder /data-service /app/data-service
EXPOSE 8083
ENTRYPOINT ["/app/data-service"]
