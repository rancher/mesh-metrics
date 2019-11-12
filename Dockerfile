FROM golang:1.13-alpine as builder

LABEL maintainer="Dax McDonald <dax@rancher.com>"

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN go build -o prom-scraper .


ENTRYPOINT [ "./main" ]

FROM alpine:3.9 as production
COPY --from=builder /app/prom-scraper /usr/bin/ 
ENTRYPOINT [ "prom-scraper" ]