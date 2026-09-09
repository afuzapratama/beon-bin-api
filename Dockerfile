FROM golang:1.26.8-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/api ./cmd/api

# ---

FROM alpine:3.23
RUN apk add --no-cache ca-certificates tzdata
RUN addgroup -S app && adduser -S -G app app
COPY --from=builder --chown=app:app /bin/api /api

EXPOSE 8080
USER app
ENTRYPOINT ["/api"]
