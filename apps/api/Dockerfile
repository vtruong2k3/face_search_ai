FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -o /api ./cmd/api

FROM alpine:3.22
RUN adduser -D -H app
USER app
COPY --from=builder /api /api
EXPOSE 8080
ENTRYPOINT ["/api"]
