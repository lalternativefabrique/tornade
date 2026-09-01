FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/tornade ./cmd/tornade

# Chromium plus the system libraries it needs is the hard part of this image;
# the Playwright base ships that set already resolved and maintained upstream.
# Only Chromium is used — this stage runs no Node.
FROM mcr.microsoft.com/playwright:v1.62.1-noble
WORKDIR /app
COPY --from=build /out/tornade /usr/local/bin/tornade

# GStreamer carries CVE-2025-3887 (H265 parsing, remote code execution) with no
# fixed version published, which fails the publish scan. It is Chromium's video
# decoding path: this service renders HTML and reads no media, so the
# dependency buys nothing here and removing it beats ignoring the finding.
RUN apt-get remove -y --purge \
      gstreamer1.0-plugins-bad \
      libgstreamer-plugins-bad1.0-0 \
    && apt-get autoremove -y \
    && rm -rf /var/lib/apt/lists/*

# This service renders arbitrary third-party JavaScript; a compromised render
# should not run as root in its own container.
RUN groupadd -r tornade && useradd -r -g tornade -G audio,video tornade \
    && mkdir -p /home/tornade && chown -R tornade:tornade /home/tornade /app
USER tornade

# chromedp looks for a browser on PATH; the Playwright image keeps its
# Chromium under /ms-playwright instead.
ENV CHROMIUM_PATH=/ms-playwright/chromium-1234/chrome-linux64/chrome

EXPOSE 8080
CMD ["tornade"]
