FROM golang:1.24-alpine AS base
EXPOSE 8085
WORKDIR /pps

FROM base AS dev
COPY payments-service/ .
# RUN go mod download

RUN go install github.com/air-verse/air@latest
COPY payments-service/ .

ENTRYPOINT ["air", "-c", "payments-service/.air.toml"]

FROM base AS prod
COPY . .
COPY go.mod .

# RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /main .

CMD ["/main"]