# syntax=docker/dockerfile:1

FROM golang:1.26.10-alpine@sha256:87c65a688975c0b1d4585d204874c45ca78354670e3a5f11368c5a492303d696 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
COPY internal/ ./internal/

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/fhir-health-service .

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab AS final

COPY --from=build /out/fhir-health-service /fhir-health-service

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/fhir-health-service"]
