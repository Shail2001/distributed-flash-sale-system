FROM --platform=$BUILDPLATFORM golang:1.21-alpine AS builder

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/waiting-room .

FROM gcr.io/distroless/static-debian12

WORKDIR /app
COPY --from=builder /out/waiting-room /app/waiting-room

ENTRYPOINT ["/app/waiting-room"]
