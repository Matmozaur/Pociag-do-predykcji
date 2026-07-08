FROM golang:1.25.0-alpine3.22 AS builder
WORKDIR /src

COPY data-service/go.mod data-service/go.sum ./data-service/
COPY shared/go.mod ./shared/go.mod
RUN cd data-service; go mod download

COPY data-service ./data-service
COPY shared ./shared

RUN cd data-service; CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /data-service ./cmd

FROM alpine:3.22.1
RUN adduser -D -u 1001 appuser
USER 1001
WORKDIR /app
COPY --from=builder /data-service /app/data-service
EXPOSE 8083
ENTRYPOINT ["/app/data-service"]
