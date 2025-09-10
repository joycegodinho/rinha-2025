FROM golang:1.25-alpine AS base
EXPOSE 9999
WORKDIR /lbb
FROM base AS builder
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -extldflags '-static' -buildid=" \
    -trimpath \
    -o /main .

FROM base AS dev
COPY load-balancer-backend/ .
RUN go mod download
RUN go install github.com/air-verse/air@latest
COPY load-balancer-backend/ .
ENTRYPOINT ["air", "-c", "load-balancer-backend/.air.toml"]

FROM alpine:3.20 AS prod
WORKDIR /lbb
RUN apk add --no-cache ca-certificates
COPY --from=builder /main /main
CMD ["/main"]