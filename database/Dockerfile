FROM golang:1.24-alpine AS base
EXPOSE 8085
WORKDIR /db

FROM base AS dev
COPY database/ .
# RUN go mod download

RUN go install github.com/air-verse/air@latest
COPY database/ .

ENTRYPOINT ["air", "-c", "database/.air.toml"]

FROM base AS prod
COPY . .
COPY go.mod .

# RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /main .

CMD ["/main"]