FROM golang:1.26.2-alpine AS builder

WORKDIR /app
COPY ./go.prod.mod ./go.mod
COPY ./go.sum .
COPY *.go .
RUN go get github.com/kmsec-uk/dochunt@main
RUN go mod download
RUN GOEXPERIMENT=jsonv2 go build -v -o /app/urlscangetdocs ./*.go

FROM alpine:3.23
COPY --from=builder /app/urlscangetdocs /app/urlscangetdocs
RUN addgroup -g 101 -S urlscan && adduser -h /app -u 1000 -D urlscan -G urlscan
USER urlscan
CMD ["/app/urlscangetdocs", "--db", "/app/data/store.db", "--apifile", "/run/secrets/urlscan_token"]