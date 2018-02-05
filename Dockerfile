FROM golang:alpine AS build

RUN apk add --no-cache git

ADD main.go /go/src/lora-influx-bridge/
RUN go get -v lora-influx-bridge

FROM alpine:latest

COPY --from=build /go/bin/lora-influx-bridge /

ENTRYPOINT ["/lora-influx-bridge"]
