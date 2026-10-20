FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server && CGO_ENABLED=0 go build -o /out/lab ./cmd/lab

FROM debian:bookworm-slim
COPY --from=build /out/ /usr/local/bin/
