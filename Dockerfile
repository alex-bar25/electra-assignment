FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /station-server ./cmd/server

FROM scratch

COPY --from=build /station-server /station-server

EXPOSE 8080

ENTRYPOINT ["/station-server"]
