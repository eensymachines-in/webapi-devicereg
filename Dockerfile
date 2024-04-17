# FROM kneerunjun/gogingonic:latest
FROM golang:1.21.8-alpine3.19

RUN apk add git
RUN mkdir -p /usr/eensy/devicereg /var/log/eensy/devicereg /usr/bin/eensy/devicereg
WORKDIR /usr/eensy/devicereg
RUN chmod -R +x /usr/bin/eensy/devicereg

COPY go.mod .
COPY go.sum .
RUN go mod download 
COPY . .

RUN go build -o /usr/bin/eensy/devicereg/devicereg .
ENTRYPOINT /usr/bin/eensy/devicereg/devicereg
