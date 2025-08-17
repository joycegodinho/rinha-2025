# FROM golang:1.24-alpine AS base
# WORKDIR /pps
# FROM base AS builder
# COPY go.mod go.sum ./
# RUN go mod download
# COPY . .
# # The output will be a static binary named 'main' in the /pps directory.
# RUN CGO_ENABLED=0 GOOS=linux go build -o /main .

# FROM base AS dev
# COPY payments-service/go.mod payments-service/go.sum ./
# RUN go mod download
# RUN go install github.com/air-verse/air@latest
# COPY payments-service/ .
# # The volume mounts the project root to /pps.
# # So the .air.toml file is at /pps/payments-service/.air.toml
# ENTRYPOINT ["air", "-c", "payments-service/.air.toml"]

# FROM golang:1.24-alpine AS prod
# WORKDIR /pps
# COPY --from=builder /main /main
# CMD ["/main"]

#################################
FROM golang:1.24-alpine AS base
WORKDIR /pps
FROM base AS builder
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -extldflags '-static' -buildid=" \
    -trimpath \
    -o /main .

# RUN CGO_ENABLED=0 GOOS=linux go build \
#     -ldflags="-s -w -buildid=" \
#     -trimpath \
#     -o /main .

FROM base AS dev
COPY payments-service/go.mod payments-service/go.sum ./
RUN go mod download
RUN go install github.com/air-verse/air@latest
COPY payments-service/ .
# The volume mounts the project root to /pps.
# So the .air.toml file is at /pps/payments-service/.air.toml
ENTRYPOINT ["air", "-c", "payments-service/.air.toml"]

FROM alpine:3.20 AS prod
WORKDIR /pps
RUN apk add --no-cache ca-certificates
COPY --from=builder /main /main
CMD ["/main"]