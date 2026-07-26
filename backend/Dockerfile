FROM golang:1.22-alpine AS builder

WORKDIR /src
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=$GOPROXY

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/delivery-api ./cmd/server

FROM alpine:3.20

WORKDIR /app
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/delivery-api /app/delivery-api

ENV APP_ENV=production
ENV HTTP_ADDR=:8080
EXPOSE 8080

CMD ["/app/delivery-api"]
