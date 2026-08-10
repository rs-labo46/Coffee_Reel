FROM golang:1.26.2 AS base

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

FROM base AS api

EXPOSE 8080

CMD ["go", "run", "."]

FROM base AS worker

RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg ca-certificates && rm -rf /var/lib/apt/lists/*

CMD ["go", "run", "./cmd/worker"]

FROM worker AS render

RUN go build -trimpath -o /usr/local/bin/coffee-reel-api . \
    && go build -trimpath -o /usr/local/bin/coffee-reel-worker ./cmd/worker

EXPOSE 10000

CMD ["/usr/local/bin/coffee-reel-api"]
