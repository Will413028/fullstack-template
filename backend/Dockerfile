# syntax=docker/dockerfile:1
# One image: `app serve` by default; the compose migrate service runs `app migrate`, which drives the bundled Atlas.
# Base images are pinned by digest; update the tag and the digest together.

FROM --platform=$BUILDPLATFORM golang:1.27.3-trixie@sha256:1896654cfc5c2b5ad74c07f29cb36f5d2d1cbed42e3054070fbfe444e521e83c AS build
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
FROM --platform=$BUILDPLATFORM golang:1.27.3-trixie@sha256:1896654cfc5c2b5ad74c07f29cb36f5d2d1cbed42e3054070fbfe444e521e83c AS atlas
ARG TARGETARCH
ARG ATLAS_VERSION=v1.3.3
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN case "$TARGETARCH" in \
      amd64) sum=7bddc9a0cebbb1f3e21cdd5f5811d346850e481e3b7c85e31206613d1f63f068 ;; \
      arm64) sum=546de518d9875e4fed991b917295395dc5400aa26922fa4cd3fbce33afdb75be ;; \
      *) echo "unsupported architecture $TARGETARCH"; exit 1 ;; \
    esac \
 && curl -sSfL -o /atlas "https://release.ariga.io/atlas/atlas-community-linux-${TARGETARCH}-${ATLAS_VERSION}" \
 && echo "$sum  /atlas" | sha256sum -c - \
 && chmod 0555 /atlas

FROM gcr.io/distroless/static-debian13:nonroot@sha256:d62067879de890a1def9da0e7766b7d20810a5e5555cef8506b752437d7a3e88
WORKDIR /app
COPY --from=build /out/app /usr/local/bin/app
COPY --from=atlas /atlas /usr/local/bin/atlas
COPY migrations migrations
# distroless's nonroot user, by number so a runtime can verify it is not root without reading /etc/passwd
USER 65532:65532
EXPOSE 8000
ENTRYPOINT ["/usr/local/bin/app"]
CMD ["serve"]
