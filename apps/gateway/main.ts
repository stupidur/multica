import "reflect-metadata";
import type { INestApplication } from "@nestjs/common";
import { NestFactory } from "@nestjs/core";
import { Logger } from "nestjs-pino";
import { AppModule } from "./app.module.js";

// Process-level error handlers — prevent unhandled errors from crashing the gateway.
process.on("unhandledRejection", (reason) => {
  console.error("[Gateway] Unhandled rejection:", reason);
});

process.on("uncaughtException", (err) => {
  console.error("[Gateway] Uncaught exception:", err);
});

let app: INestApplication;

async function gracefulShutdown(): Promise<void> {
  console.log("[Gateway] Shutting down gracefully...");
  await app?.close();
  process.exit(0);
}

process.on("SIGTERM", gracefulShutdown);
process.on("SIGINT", gracefulShutdown);

console.log("[Gateway] Starting bootstrap...");

async function bootstrap(): Promise<void> {
  console.log("[Gateway] Creating NestFactory...");
  try {
    app = await NestFactory.create(AppModule, { bufferLogs: true, abortOnError: false });
    console.log("[Gateway] NestFactory created");

    app.useLogger(app.get(Logger));
    app.enableCors();

    const port = process.env["PORT"] ?? 3000;
    console.log(`[Gateway] Listening on port ${port}...`);
    await app.listen(port);

    const logger = app.get(Logger);
    logger.log(`Gateway is running on http://localhost:${port}`);
    logger.log(`WebSocket endpoint: ws://localhost:${port}/ws`);
  } catch (err) {
    console.error("[Gateway] Error during bootstrap:", err);
    throw err;
  }
}

bootstrap().catch((err) => {
  console.error("Failed to start gateway:", err);
  process.exit(1);
});
