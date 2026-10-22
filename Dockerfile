FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
RUN CGO_ENABLED=0 go build -tags netgo -ldflags "-s -w" -o /out/matan-trainer ./cmd/server

FROM alpine:3.22

RUN adduser -D -H -u 10001 app
COPY --from=build /out/matan-trainer /usr/local/bin/matan-trainer

USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/matan-trainer"]
