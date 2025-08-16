FROM golang:1.24-alpine AS base
WORKDIR /wu
FROM base AS builder
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /main .

FROM golang:1.24-alpine AS prod
WORKDIR /wu
COPY --from=builder /main /main
CMD ["/main"]