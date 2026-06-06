export interface LarkOAuthState {
  nonce: string;
  next: string;
  cliCallback: string;
  cliState: string;
}

export function parseState(raw: string): LarkOAuthState | null {
  try {
    const parsed = JSON.parse(raw) as {
      nonce?: unknown;
      next?: unknown;
      cli_callback?: unknown;
      cli_state?: unknown;
    };
    if (typeof parsed.nonce !== "string" || parsed.nonce === "") return null;
    return {
      nonce: parsed.nonce,
      next: typeof parsed.next === "string" ? parsed.next : "",
      cliCallback: typeof parsed.cli_callback === "string" ? parsed.cli_callback : "",
      cliState: typeof parsed.cli_state === "string" ? parsed.cli_state : "",
    };
  } catch {
    return null;
  }
}

export function buildStatePayload(state: LarkOAuthState): string {
  return JSON.stringify({
    nonce: state.nonce,
    next: state.next,
    cli_callback: state.cliCallback,
    cli_state: state.cliState,
  });
}

export function buildPostAuthRedirect(appOrigin: string, state: LarkOAuthState): URL {
  const target = new URL(state.cliCallback ? "/login" : "/onboarding", appOrigin);
  if (state.cliCallback) {
    target.searchParams.set("cli_callback", state.cliCallback);
    if (state.cliState) target.searchParams.set("cli_state", state.cliState);
    if (state.next) target.searchParams.set("next", state.next);
    return target;
  }
  if (state.next) target.searchParams.set("next", state.next);
  return target;
}
