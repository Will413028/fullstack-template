# syntax=docker/dockerfile:1
# One image: `app serve` by default; the compose migrate service runs `atlas migrate apply` from the same image.
# Base images are pinned by digest; update the tag and the digest together.

FROM --platform=$BUILDPLATFORM golang:1.27.2-trixie@sha256:47667934874913a449b96faa7ef4777fa285d9ec29126ee80ed61c45b7cccf0e AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd cmd
COPY internal internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/app ./cmd/app

# Migrations are applied with the Atlas community build (Apache-2.0); the standard build that generates them is
# used only on developer machines and in CI (backend/Makefile). Checksums come from release.ariga.io.
FROM --platform=$BUILDPLATFORM golang:1.27.2-trixie@sha256:47667934874913a449b96faa7ef4777fa285d9ec29126ee80ed61c45b7cccf0e AS atlas
ARG TARGETARCH
ARG ATLAS_VERSION=v1.1.3
RUN case "$TARGETARCH" in \
      amd64) sum=db596f9e39c33aae3bcbed674dab2255dd20b5e7bee29f7ed83413f6dd7b5047 ;; \
      arm64) sum=511996bddf663d14a243d3e95ebb0d3d1d0a2aa89abb2fc85810a12cad7c390f ;; \
      *) echo "unsupported architecture $TARGETARCH"; exit 1 ;; \
    esac \
 && curl -sSfL -o /atlas "https://release.ariga.io/atlas/atlas-community-linux-${TARGETARCH}-${ATLAS_VERSION}" \
 && echo "$sum  /atlas" | sha256sum -c - \
 && chmod 0555 /atlas

FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
WORKDIR /app
COPY --from=build /out/app /usr/local/bin/app
COPY --from=atlas /atlas /usr/local/bin/atlas
COPY atlas.hcl ./
COPY migrations migrations
USER nonroot
EXPOSE 8000
ENTRYPOINT ["/usr/local/bin/app"]
CMD ["serve"]
