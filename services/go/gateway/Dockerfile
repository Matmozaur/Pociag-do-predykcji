FROM golang:1.25 AS builder
WORKDIR /src

COPY gateway/go.mod gateway/go.sum ./gateway/
COPY shared/go.mod ./shared/go.mod
RUN cd gateway; go mod download

COPY gateway ./gateway
COPY shared ./shared

RUN cd gateway; CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /gateway ./cmd/main.go

FROM gcr.io/distroless/base-debian12:nonroot
COPY --from=builder /gateway /gateway
EXPOSE 8084
ENTRYPOINT ["/gateway"]
