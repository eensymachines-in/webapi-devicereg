# FROM kneerunjun/gogingonic:latest
FROM golang:1.21.8-alpine3.19

ARG SRC
ARG LOG
ARG RUN
ARG ETC 
ARG BIN
ARG APPNAME
RUN apk add git
RUN mkdir -p ${SRC} && mkdir -p ${LOG} && mkdir -p ${RUN} && mkdir -p ${ETC} && mkdir -p ${BIN}
WORKDIR ${SRC}
RUN chmod -R +x ${BIN}

COPY go.mod .
COPY go.sum .
RUN go mod download 
COPY . .

RUN go build -o ${BIN}/${APPNAME} .