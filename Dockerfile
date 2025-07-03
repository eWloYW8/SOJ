FROM docker.io/library/golang:bookworm AS build


RUN apt-get update && apt-get install -y build-essential && go env -w GOPROXY=https://goproxy.cn,direct

COPY go.mod go.sum /go/src/github.com/mrhaoxx/SOJ/
RUN cd /go/src/github.com/mrhaoxx/SOJ && go mod download

COPY . /go/src/github.com/mrhaoxx/SOJ

RUN cd /go/src/github.com/mrhaoxx/SOJ && go build -o /soj

FROM debian:bookworm AS runtime


COPY --from=build /soj /soj

ENTRYPOINT ["/soj"]
