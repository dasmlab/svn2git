# Multi-stage build for the svn_to_git CLI

FROM golang:1.22 AS builder
WORKDIR /src
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o /out/svn_to_git ./cmd/svn_to_git

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
COPY --from=builder /out/svn_to_git /usr/local/bin/svn_to_git
USER app
ENTRYPOINT ["svn_to_git"]

