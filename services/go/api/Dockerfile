FROM golang:1.25.0-alpine3.22 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /api ./cmd

FROM alpine:3.22.1
RUN adduser -D -u 1001 appuser
USER 1001
COPY --from=builder /api /app/api
EXPOSE 8080
ENTRYPOINT ["/app/api"]
