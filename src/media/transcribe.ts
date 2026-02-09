/**
 * Audio transcription via OpenAI Whisper API.
 *
 * Called by ChannelManager before the message reaches the Agent,
 * so the Agent only ever sees text.
 */

import { readFile } from "node:fs/promises";
import { basename } from "node:path";
import { credentialManager } from "../agent/credentials.js";

/**
 * Transcribe an audio file using OpenAI Whisper API.
 *
 * @param filePath - Local path to the audio file
 * @returns Transcribed text, or null if no API key configured
 */
export async function transcribeAudio(filePath: string): Promise<string | null> {
  const config = credentialManager.getLlmProviderConfig("openai");
  const apiKey = config?.apiKey;
  if (!apiKey) return null;

  const fileBuffer = await readFile(filePath);
  const fileName = basename(filePath);

  // Build multipart form data manually (no external dependency)
  const boundary = `----FormBoundary${Date.now()}`;
  const parts: Buffer[] = [];

  // file field
  parts.push(Buffer.from(
    `--${boundary}\r\nContent-Disposition: form-data; name="file"; filename="${fileName}"\r\nContent-Type: application/octet-stream\r\n\r\n`,
  ));
  parts.push(fileBuffer);
  parts.push(Buffer.from("\r\n"));

  // model field
  parts.push(Buffer.from(
    `--${boundary}\r\nContent-Disposition: form-data; name="model"\r\n\r\nwhisper-1\r\n`,
  ));

  // closing boundary
  parts.push(Buffer.from(`--${boundary}--\r\n`));

  const body = Buffer.concat(parts);

  const res = await fetch("https://api.openai.com/v1/audio/transcriptions", {
    method: "POST",
    headers: {
      Authorization: `Bearer ${apiKey}`,
      "Content-Type": `multipart/form-data; boundary=${boundary}`,
    },
    body,
  });

  if (!res.ok) {
    const errText = await res.text().catch(() => "");
    throw new Error(`Whisper API error: HTTP ${res.status} ${errText}`);
  }

  const result = (await res.json()) as { text: string };
  return result.text;
}
