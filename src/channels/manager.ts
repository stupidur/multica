/**
 * Channel Manager — bridges messaging channels to the Hub's agent.
 *
 * Design: One Hub, one Agent. Channels are just alternative input/output surfaces.
 * - Incoming: channel message → agent.write(text)  (same as desktop/gateway)
 * - Outgoing: agent reply → check lastRoute → forward to originating channel
 *
 * Uses "last route" pattern: whoever sent the last message gets the reply.
 */

import { readFile } from "node:fs/promises";
import type { Hub } from "../hub/hub.js";
import type {
  ChannelPlugin,
  ChannelMessage,
  ChannelAccountState,
  DeliveryContext,
} from "./types.js";
import { listChannels } from "./registry.js";
import { loadChannelsConfig } from "./config.js";
import { MessageAggregator, DEFAULT_CHUNKER_CONFIG } from "../hub/message-aggregator.js";
import type { AsyncAgent } from "../agent/async-agent.js";
import { transcribeAudio } from "../media/transcribe.js";

interface AccountHandle {
  channelId: string;
  accountId: string;
  abortController: AbortController;
  state: ChannelAccountState;
}

/** Tracks where the last message came from, so replies go back there. */
interface LastRoute {
  plugin: ChannelPlugin;
  deliveryCtx: DeliveryContext;
}

export class ChannelManager {
  private readonly hub: Hub;
  /** Running accounts keyed by "channelId:accountId" */
  private readonly accounts = new Map<string, AccountHandle>();
  /** Where the last channel message came from (reply target) */
  private lastRoute: LastRoute | null = null;
  /** Unsubscribe function for the agent subscriber */
  private agentUnsubscribe: (() => void) | null = null;
  /** Current aggregator for buffering streaming responses */
  private aggregator: MessageAggregator | null = null;
  /** Typing indicator interval (repeats every 5s to keep Telegram typing visible) */
  private typingTimer: ReturnType<typeof setInterval> | null = null;

  constructor(hub: Hub) {
    this.hub = hub;
  }

  /** Start all configured channel accounts */
  async startAll(): Promise<void> {
    console.log("[Channels] Starting all channels...");
    const config = loadChannelsConfig();
    const plugins = listChannels();

    if (plugins.length === 0) {
      console.log("[Channels] No plugins registered");
      return;
    }

    for (const plugin of plugins) {
      const accountIds = plugin.config.listAccountIds(config);
      if (accountIds.length === 0) {
        console.log(`[Channels] Skipping ${plugin.id} (not configured)`);
        continue;
      }

      for (const accountId of accountIds) {
        const account = plugin.config.resolveAccount(config, accountId);
        if (!account || !plugin.config.isConfigured(account)) {
          console.log(`[Channels] Skipping ${plugin.id}:${accountId} (incomplete config)`);
          continue;
        }
        await this.startAccount(plugin.id, accountId, account);
      }
    }

    // Subscribe to the Hub's agent for outbound routing
    this.subscribeToAgent();
  }

  /** Start a specific channel account */
  private async startAccount(
    channelId: string,
    accountId: string,
    accountConfig: Record<string, unknown>,
  ): Promise<void> {
    const key = `${channelId}:${accountId}`;
    if (this.accounts.has(key)) {
      console.warn(`[Channels] ${key} is already running`);
      return;
    }

    const plugin = listChannels().find((p) => p.id === channelId);
    if (!plugin) {
      console.error(`[Channels] Plugin "${channelId}" not found`);
      return;
    }

    const abortController = new AbortController();
    const handle: AccountHandle = {
      channelId,
      accountId,
      abortController,
      state: { channelId, accountId, status: "starting" },
    };
    this.accounts.set(key, handle);

    console.log(`[Channels] Starting ${key}`);

    try {
      const startPromise = plugin.gateway.start(
        accountId,
        accountConfig,
        (message: ChannelMessage) => {
          this.routeIncoming(plugin, accountId, message);
        },
        abortController.signal,
      );

      await Promise.race([
        startPromise,
        new Promise<void>((resolve) => setTimeout(resolve, 3000)),
      ]);

      handle.state = { channelId, accountId, status: "running" };
      console.log(`[Channels] ${key} is running`);
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : String(err);
      handle.state = { channelId, accountId, status: "error", error: errorMsg };
      console.error(`[Channels] Failed to start ${key}: ${errorMsg}`);
    }
  }

  /** Get the Hub's current agent (the first active one) */
  private getHubAgent(): AsyncAgent | undefined {
    const agentIds = this.hub.listAgents();
    if (agentIds.length === 0) {
      console.warn("[Channels] No agent available in Hub");
      return undefined;
    }
    const agent = this.hub.getAgent(agentIds[0]!);
    return agent;
  }

  /**
   * Subscribe to the Hub's agent events (once, persistent).
   * When AI replies and lastRoute points to a channel, forward the reply there.
   */
  private subscribeToAgent(): void {
    const agent = this.getHubAgent();
    if (!agent) {
      console.warn("[Channels] No agent to subscribe to, channel replies will not be routed");
      return;
    }

    console.log(`[Channels] Subscribing to agent ${agent.sessionId} for outbound routing`);

    this.agentUnsubscribe = agent.subscribe((event) => {
      // No active channel route — skip (reply goes to desktop/gateway only)
      if (!this.lastRoute) return;

      // Handle agent errors — notify the channel user
      if (event.type === "agent_error") {
        this.stopTyping();
        const errorMsg = (event as { error?: string }).error ?? "Unknown error";
        console.error(`[Channels] Agent error: ${errorMsg}`);
        const route = this.lastRoute;
        if (route) {
          void route.plugin.outbound.sendText(route.deliveryCtx, `[Error] ${errorMsg}`).catch((err) => {
            console.error(`[Channels] Failed to send error to channel: ${err}`);
          });
        }
        return;
      }

      const maybeMessage = (event as { message?: { role?: string } }).message;
      const role = maybeMessage?.role;

      // Only forward assistant message events
      if (event.type === "message_start" || event.type === "message_update" || event.type === "message_end") {
        if (role !== "assistant") return;
      } else {
        // Non-message events (tool_execution etc.) — skip for channels
        return;
      }

      // Ensure aggregator exists for this response
      if (event.type === "message_start") {
        this.createAggregator();
      }

      if (this.aggregator) {
        this.aggregator.handleEvent(event);
      }

      // Clean up after response complete
      if (event.type === "message_end" && role === "assistant") {
        this.stopTyping();
        this.aggregator = null;
      }
    });
  }

  /** Create a fresh aggregator wired to the current lastRoute */
  private createAggregator(): void {
    const route = this.lastRoute;
    if (!route) return;

    const { plugin, deliveryCtx } = route;
    const chunkerConfig = plugin.chunkerConfig ?? DEFAULT_CHUNKER_CONFIG;

    this.aggregator = new MessageAggregator(
      chunkerConfig,
      async (block) => {
        try {
          console.log(`[Channels] Sending block ${block.index} (${block.text.length} chars${block.isFinal ? ", final" : ""}) → ${deliveryCtx.channel}:${deliveryCtx.conversationId}`);
          if (block.index === 0) {
            await plugin.outbound.replyText(deliveryCtx, block.text);
          } else {
            await plugin.outbound.sendText(deliveryCtx, block.text);
          }
        } catch (err) {
          console.error(`[Channels] Failed to send reply: ${err}`);
        }
      },
      () => {},
    );
  }

  /**
   * Incoming channel message → update lastRoute → forward to Hub's agent.
   */
  private routeIncoming(
    plugin: ChannelPlugin,
    accountId: string,
    message: ChannelMessage,
  ): void {
    const { conversationId, senderId, text, messageId } = message;
    console.log(
      `[Channels] Incoming: channel=${plugin.id} conv=${conversationId} sender=${senderId} text="${text.slice(0, 50)}${text.length > 50 ? "..." : ""}"`,
    );

    const agent = this.getHubAgent();
    if (!agent) {
      console.error("[Channels] No agent available, dropping message");
      return;
    }

    // Update last route — replies will go back here
    this.lastRoute = {
      plugin,
      deliveryCtx: {
        channel: plugin.id,
        accountId,
        conversationId,
        replyToMessageId: messageId,
      },
    };
    console.log(`[Channels] lastRoute updated → ${plugin.id}:${conversationId}`);
    console.log(`[Channels] Forwarding to agent ${agent.sessionId}`);

    // Show typing indicator while agent processes
    this.startTyping();

    // Handle media messages
    if (message.media && plugin.downloadMedia) {
      void this.routeMedia(plugin, accountId, message, agent);
    } else {
      agent.write(text);
    }
  }

  /** Download media file, process it, and forward result to agent */
  private async routeMedia(
    plugin: ChannelPlugin,
    accountId: string,
    message: ChannelMessage,
    agent: AsyncAgent,
  ): Promise<void> {
    const media = message.media!;

    try {
      const filePath = await plugin.downloadMedia!(media.fileId, accountId);

      if (media.type === "image") {
        // Images: pass directly to LLM as ImageContent
        const buffer = await readFile(filePath);
        const base64 = buffer.toString("base64");
        const mimeType = media.mimeType ?? "image/jpeg";
        const caption = media.caption || "User sent an image.";
        agent.writeWithImages(caption, [{ type: "image", data: base64, mimeType }]);
      } else if (media.type === "audio") {
        // Audio: transcribe via Whisper API before reaching agent
        const transcript = await transcribeAudio(filePath);
        if (transcript) {
          const parts = ["[Voice Message]", `Transcript: ${transcript}`];
          if (media.caption) parts.push(`Caption: ${media.caption}`);
          agent.write(parts.join("\n"));
        } else {
          // No API key configured — fall back to file path
          const parts = ["[audio message received]", `File: ${filePath}`];
          if (media.mimeType) parts.push(`Type: ${media.mimeType}`);
          if (media.duration) parts.push(`Duration: ${media.duration}s`);
          if (media.caption) parts.push(`Caption: ${media.caption}`);
          agent.write(parts.join("\n"));
        }
      } else {
        // Video/document: tell agent the file path
        const parts: string[] = [];
        parts.push(`[${media.type} message received]`);
        parts.push(`File: ${filePath}`);
        if (media.mimeType) parts.push(`Type: ${media.mimeType}`);
        if (media.duration) parts.push(`Duration: ${media.duration}s`);
        if (media.caption) parts.push(`Caption: ${media.caption}`);
        agent.write(parts.join("\n"));
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      console.error(`[Channels] Failed to process media: ${msg}`);
      agent.write(message.text || `[Failed to process ${media.type}]`);
    }
  }

  /** Start sending typing indicators (repeats every 5s until stopped) */
  private startTyping(): void {
    this.stopTyping();
    const route = this.lastRoute;
    if (!route?.plugin.outbound.sendTyping) return;

    const send = () => route.plugin.outbound.sendTyping!(route.deliveryCtx).catch(() => {});
    void send();
    this.typingTimer = setInterval(send, 5000);
  }

  /** Stop typing indicator interval */
  private stopTyping(): void {
    if (this.typingTimer) {
      clearInterval(this.typingTimer);
      this.typingTimer = null;
    }
  }

  /** Stop all running channel accounts */
  stopAll(): void {
    console.log("[Channels] Stopping all channels...");
    this.stopTyping();
    if (this.agentUnsubscribe) {
      this.agentUnsubscribe();
      this.agentUnsubscribe = null;
    }
    for (const [key, handle] of this.accounts) {
      handle.abortController.abort();
      handle.state = { ...handle.state, status: "stopped" };
      console.log(`[Channels] Stopped ${key}`);
    }
    this.accounts.clear();
    this.lastRoute = null;
    this.aggregator = null;
  }

  /** Clear the last route (e.g. when desktop user sends a message) */
  clearLastRoute(): void {
    if (this.lastRoute) {
      this.stopTyping();
      console.log("[Channels] lastRoute cleared (non-channel message received)");
      this.lastRoute = null;
    }
  }

  /** Get status of all accounts */
  listAccountStates(): ChannelAccountState[] {
    return Array.from(this.accounts.values()).map((h) => ({ ...h.state }));
  }
}
