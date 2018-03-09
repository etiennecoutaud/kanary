FROM golang:1.9
COPY . /go/src/github.com/etiennecoutaud/kanary
WORKDIR /go/src/github.com/etiennecoutaud/kanary/cmd/kanary-operator
RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s" -o kanary-operator

FROM scratch
COPY --from=0 /go/src/github.com/etiennecoutaud/kanary/cmd/kanary-operator/kanary-operator /
LABEL app.language=golang app.name=kanary-operator
EXPOSE 8080
ENTRYPOINT ["/kanary-operator", "-logtostderr",  "-v=2"]