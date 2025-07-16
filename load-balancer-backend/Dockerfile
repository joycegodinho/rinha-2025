FROM golang:1.24-alpine AS base
EXPOSE 8085
WORKDIR /lbb

FROM base AS dev
COPY load-balancer-backend/ .
# RUN go mod download

RUN go install github.com/air-verse/air@latest
COPY load-balancer-backend/ .

ENTRYPOINT ["air", "-c", "load-balancer-backend/.air.toml"]

FROM base AS prod
COPY . .
COPY go.mod .

# RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /main .

CMD ["/main"]