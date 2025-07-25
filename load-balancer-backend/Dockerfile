FROM golang:1.24-alpine AS base
EXPOSE 9999
WORKDIR /lbb
FROM base AS builder
COPY . .
COPY go.mod go.sum ./
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /main ./main.go
FROM base AS dev
COPY load-balancer-backend/ .
# RUN go mod download
RUN go install github.com/air-verse/air@latest
COPY load-balancer-backend/ .
ENTRYPOINT ["air", "-c", "load-balancer-backend/.air.toml"]

FROM builder AS prod
# Copy the compiled binary from the builder stage.
# COPY --from=builder /lbb/main .
CMD ["/main"]