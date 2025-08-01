FROM golang:1.24-alpine AS base
EXPOSE 9999
WORKDIR /lbb
FROM base AS builder
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /main .
FROM base AS dev
COPY load-balancer-backend/ .
RUN go mod download
RUN go install github.com/air-verse/air@latest
COPY load-balancer-backend/ .
ENTRYPOINT ["air", "-c", "load-balancer-backend/.air.toml"]

FROM golang:1.24-alpine AS prod
WORKDIR /lbb
COPY --from=builder /main /main
CMD ["/main"]