# syntax=docker/dockerfile:1

FROM node:22-alpine AS frontend-build

WORKDIR /src/frontend

COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

COPY frontend/ ./
RUN npm run build


FROM golang:1.25-alpine AS backend-build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cca .


FROM alpine:3.21

RUN apk add --no-cache ca-certificates socat

WORKDIR /app

COPY --from=backend-build /out/cca /app/cca
COPY --from=frontend-build /src/frontend/dist /app/frontend/dist
COPY admin_static /app/admin_static
COPY dev/docker-entrypoint.sh /usr/local/bin/cca-docker-entrypoint

RUN chmod 0755 /usr/local/bin/cca-docker-entrypoint

EXPOSE 8192

ENTRYPOINT ["/usr/local/bin/cca-docker-entrypoint"]
