FROM golang:1.25 AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /gateway ./cmd/main.go

FROM gcr.io/distroless/base-debian12:nonroot
COPY --from=builder /gateway /gateway
EXPOSE 8084
ENTRYPOINT ["/gateway"]
