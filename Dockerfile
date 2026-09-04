FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api

# stage 2

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

ENV NODE_ENV=production

WORKDIR /app

COPY --from=builder /out/api ./api

EXPOSE 4002

USER nobody

CMD ["./api"]
