FROM golang:1.26.1-alpine AS builder

WORKDIR /app
COPY ./go.prod.mod ./go.mod
COPY ./go.sum .
COPY *.go .
COPY static static
COPY tmpl tmpl
RUN go mod download
RUN go build -v -o /app/srv ./*.go

FROM alpine:3.23
COPY --from=builder /app/srv /app/srv
RUN addgroup -g 101 -S web && adduser -h /app -u 1001 -D web -G web
USER web
CMD ["/app/srv", "--db", "/app/store.db"]