# syntax=docker/dockerfile:1
# Build multi-stage: la imagen final no incluye compilador ni código fuente.
FROM golang:1.26-alpine AS build
WORKDIR /src

# Primero solo los módulos: aprovecha la caché de capas de Docker.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# -trimpath: sin rutas locales en el binario
# -ldflags="-s -w": binario más chico (sin tabla de símbolos)
# CGO_ENABLED=0: binario estático, corre en distroless/scratch
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/servidor ./18-produccion

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/servidor /servidor
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/servidor"]
