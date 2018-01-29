FROM golang:1.9.2
MAINTAINER "Maxim Krivich"
RUN mkdir /server
ADD . /server/
WORKDIR /server
RUN go build -o main .
ENTRYPOINT ["./main"]