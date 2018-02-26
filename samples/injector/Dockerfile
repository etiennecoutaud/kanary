FROM golang:1.9 as builder
WORKDIR /go/src/app/
COPY . /go/src/app
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

FROM scratch
WORKDIR /root/
COPY --from=builder /go/src/app/main .
CMD ["./main"]
