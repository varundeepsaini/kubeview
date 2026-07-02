# Build a static binary, then ship it on a distroless base so the runtime
# image carries no shell, package manager, or libc.
FROM golang:1.26.4 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /kubeview-backend .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /kubeview-backend /kubeview-backend

EXPOSE 5501
USER nonroot
ENTRYPOINT ["/kubeview-backend"]
