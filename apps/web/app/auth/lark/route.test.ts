import { describe, expect, it } from "vitest";
import {
  buildPostAuthRedirect,
  buildStatePayload,
  parseState,
  type LarkOAuthState,
} from "./route-state";

describe("Lark auth route helpers", () => {
  it("round-trips cli callback fields through oauth state", () => {
    const state: LarkOAuthState = {
      nonce: "nonce-1",
      next: "/invite/123",
      cliCallback: "http://192.168.18.150:60094/callback",
      cliState: "abc123",
    };

    expect(parseState(buildStatePayload(state))).toEqual(state);
  });

  it("redirects cli oauth completions back to login with callback params", () => {
    const target = buildPostAuthRedirect("https://multica-dev.yuzuski.top", {
      nonce: "nonce-1",
      next: "",
      cliCallback: "http://192.168.18.150:60094/callback",
      cliState: "abc123",
    });

    expect(target.toString()).toBe(
      "https://multica-dev.yuzuski.top/login?cli_callback=http%3A%2F%2F192.168.18.150%3A60094%2Fcallback&cli_state=abc123",
    );
  });

  it("redirects non-cli oauth completions to onboarding", () => {
    const target = buildPostAuthRedirect("https://multica-dev.yuzuski.top", {
      nonce: "nonce-1",
      next: "",
      cliCallback: "",
      cliState: "",
    });

    expect(target.toString()).toBe("https://multica-dev.yuzuski.top/onboarding");
  });
});
