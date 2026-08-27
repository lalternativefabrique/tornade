# The official Playwright image ships Chromium plus every system library it
# needs preinstalled — building that dependency set by hand on plain
# node:alpine is what "installing Chrome in a container" is normally the
# hard part of. The version here must track package.json's playwright
# dependency: a mismatch makes Playwright refuse to launch the browser it
# finds against the version it was built for.
FROM mcr.microsoft.com/playwright:v1.62.1-noble

WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm install --omit=dev
COPY src/ src/

# The base image's default user is root; Chromium sandboxing does not need
# it, and a compromised render (this service exists to run arbitrary
# third-party JavaScript) should not run as root in its own container.
RUN groupadd -r tornade && useradd -r -g tornade -G audio,video tornade \
    && mkdir -p /home/tornade/Downloads \
    && chown -R tornade:tornade /home/tornade /app
USER tornade

ENV NODE_ENV=production
EXPOSE 8080
CMD ["node", "src/server.js"]
