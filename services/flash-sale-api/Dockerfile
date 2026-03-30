FROM --platform=linux/amd64 golang:1.21-alpine

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o server .

EXPOSE 8080

CMD ["./server"]
