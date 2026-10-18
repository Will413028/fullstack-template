# syntax=docker/dockerfile:1
# One image: `app serve` by default; the compose migrate service runs `app migrate`, which drives the bundled Atlas.
# Base images are pinned by digest; update the tag and the digest together.

FROM --platform=$BUILDPLATFORM golang:1.27.2-trixie@sha256:47667934874913a449b96faa7ef4777fa285d9ec29126ee80ed61c45b7cccf0e AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY api api
COPY cmd cmd
COPY internal internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/app ./cmd/app

# Migrations are applied with the Atlas community build (Apache-2.0); the standard build that generates them is
# used only on developer machines and in CI (backend/Makefile). Checksums come from release.ariga.io and match
# ATLAS_COMMUNITY_SHA256_* in backend/Makefile, which the integration tests use.
FROM --platform=$BUILDPLATFORM golang:1.27.2-trixie@sha256:47667934874913a449b96faa7ef4777fa285d9ec29126ee80ed61c45b7cccf0e AS atlas
ARG TARGETARCH
ARG ATLAS_VERSION=v1.3.3
RUN case "$TARGETARCH" in \
      amd64) sum=7bddc9a0cebbb1f3e21cdd5f5811d346850e481e3b7c85e31206613d1f63f068 ;; \
      arm64) sum=546de518d9875e4fed991b917295395dc5400aa26922fa4cd3fbce33afdb75be ;; \
      *) echo "unsupported architecture $TARGETARCH"; exit 1 ;; \
    esac \
 && curl -sSfL -o /atlas "https://release.ariga.io/atlas/atlas-community-linux-${TARGETARCH}-${ATLAS_VERSION}" \
 && echo "$sum  /atlas" | sha256sum -c - \
 && chmod 0555 /atlas

FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
WORKDIR /app
COPY --from=build /out/app /usr/local/bin/app
COPY --from=atlas /atlas /usr/local/bin/atlas
COPY migrations migrations
USER nonroot
EXPOSE 8000
ENTRYPOINT ["/usr/local/bin/app"]
CMD ["serve"]
