import { createMdbrainWebApp } from "./server";

if (import.meta.main) {
  const app = createMdbrainWebApp();
  const errorHandler = (error: unknown) => {
    console.error(error);
    return new Response("Internal Server Error", { status: 500 });
  };

  const consoleServer = Bun.serve({
    hostname: app.config.host,
    port: app.config.consolePort,
    fetch(request) {
      return app.fetch(request);
    },
    error: errorHandler,
  });

  const appServer =
    app.config.appPort === app.config.consolePort
      ? consoleServer
      : Bun.serve({
          hostname: app.config.host,
          port: app.config.appPort,
          fetch(request) {
            return app.fetch(request);
          },
          error: errorHandler,
        });

  console.log(`Mdbrain Bun app listening on ${appServer.url}`);
  console.log(`Mdbrain Bun console listening on ${consoleServer.url}`);
}
