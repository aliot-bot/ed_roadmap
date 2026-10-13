FROM golang:1.26.9-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /roadmap-api ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /roadmap-api /roadmap-api
EXPOSE 8080
ENTRYPOINT ["/roadmap-api"]
