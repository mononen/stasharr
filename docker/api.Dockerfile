FROM golang:1.23-alpine AS dev
WORKDIR /app
RUN apk add --no-cache git
RUN go install github.com/air-verse/air@latest
COPY go.mod go.sum ./
RUN go mod download
CMD ["air", "-c", ".air.toml"]

FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o stasharr ./cmd/stasharr

FROM alpine:3.19 AS production
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/stasharr .
EXPOSE 8080
CMD ["./stasharr"]
