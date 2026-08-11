# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
COPY internal/ ./internal/

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/fhir-health-service .

FROM gcr.io/distroless/static-debian12:nonroot AS final

COPY --from=build /out/fhir-health-service /fhir-health-service

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/fhir-health-service"]
