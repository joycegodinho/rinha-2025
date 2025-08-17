FROM golang:1.24-alpine AS base
WORKDIR /db

FROM base AS builder
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# The output will be a static binary named 'main' in the /db directory.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -extldflags '-static' -buildid=" \
    -trimpath \
    -o /main .

FROM base AS dev
COPY database/go.mod database/go.sum ./
RUN go mod download
RUN go install github.com/air-verse/air@latest
COPY database/ .
ENTRYPOINT ["air", "-c", "database/.air.toml"]

FROM alpine:3.20 AS prod
WORKDIR /db
RUN apk add --no-cache ca-certificates
COPY --from=builder /main /main
CMD ["/main"]
