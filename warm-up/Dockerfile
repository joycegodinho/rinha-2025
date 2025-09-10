FROM golang:1.25-alpine AS base
WORKDIR /wu
FROM base AS builders
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -extldflags '-static' -buildid=" \
    -trimpath \
    -o /main .

FROM alpine:3.20 AS prod
WORKDIR /wu
RUN apk add --no-cache ca-certificates
COPY --from=builder /main /main
CMD ["/main"]