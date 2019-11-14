FROM golang:1.13-alpine as builder

LABEL maintainer="Dax McDonald <dax@rancher.com>"

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN go build -o mesh-metrics .


ENTRYPOINT [ "./main" ]

FROM alpine:3.9 as production
COPY --from=builder /app/mesh-metrics /usr/bin/ 
ENTRYPOINT [ "mesh-metrics" ]