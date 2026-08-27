import { createApp } from './app.js';
import { render, closeBrowser } from './browser.js';

const PORT = process.env.PORT || 8080;

const server = createApp(render);

server.listen(PORT, () => {
  console.log(`tornade: listening on :${PORT}`);
});

// Playwright's browser process is a child of this one; an ungraceful exit
// leaves it running as an orphan under the container's PID namespace until
// the pod is killed outright.
for (const signal of ['SIGTERM', 'SIGINT']) {
  process.on(signal, async () => {
    await closeBrowser();
    server.close(() => process.exit(0));
  });
}
