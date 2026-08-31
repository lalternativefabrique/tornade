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
