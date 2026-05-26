import { NextRequest, NextResponse } from "next/server";

const LARK_OAUTH_STATE_COOKIE = "multica_lark_oauth_state";

function buildAuthorizeUrl(appId: string, redirectUri: string, state: string) {
  const url = new URL("https://accounts.feishu.cn/open-apis/authen/v1/authorize");
  url.searchParams.set("client_id", appId);
  url.searchParams.set("redirect_uri", redirectUri);
  url.searchParams.set("response_type", "code");
  url.searchParams.set("scope", "contact:user.base:readonly contact:user.email:readonly");
  url.searchParams.set("state", state);
  return url.toString();
}

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

function clearOAuthStateCookie(response: NextResponse) {
  response.cookies.set(LARK_OAUTH_STATE_COOKIE, "", {
    httpOnly: true,
    sameSite: "lax",
    path: "/auth/lark",
    maxAge: 0,
  });
}

function getSetCookies(headers: Headers) {
  const maybeGetSetCookie = headers as Headers & { getSetCookie?: () => string[] };
  if (typeof maybeGetSetCookie.getSetCookie === "function") {
    return maybeGetSetCookie.getSetCookie();
  }
  const raw = headers.get("set-cookie");
  if (!raw) return [];
  return raw.split(/,(?=\s*[^;]+=)/g);
}

function allowOAuthRedirectCookie(cookie: string) {
  return cookie.replace(/SameSite=Strict/gi, "SameSite=Lax");
}

export async function GET(req: NextRequest) {
  const appId = process.env.LARK_APP_ID;
  const redirectUri = process.env.LARK_REDIRECT_URI || `${req.nextUrl.origin}/auth/lark`;
  if (!appId) {
    console.warn("[lark-auth] missing LARK_APP_ID");
    return NextResponse.json({ error: "Lark login is not configured" }, { status: 503 });
  }

  const code = req.nextUrl.searchParams.get("code") || "";
  if (!code) {
    const next = req.nextUrl.searchParams.get("next") || "";
    const cliCallback = req.nextUrl.searchParams.get("cli_callback") || "";
    const cliState = req.nextUrl.searchParams.get("cli_state") || "";
    const nonce = crypto.randomUUID();
    const state = buildStatePayload({ nonce, next, cliCallback, cliState });
    console.info("[lark-auth] redirecting to Feishu authorize", { redirectUri, hasNext: next !== "" });
    const response = NextResponse.redirect(buildAuthorizeUrl(appId, redirectUri, state));
    response.cookies.set(LARK_OAUTH_STATE_COOKIE, nonce, {
      httpOnly: true,
      sameSite: "lax",
      secure: req.nextUrl.protocol === "https:",
      path: "/auth/lark",
      maxAge: 10 * 60,
    });
    return response;
  }

  const state = parseState(req.nextUrl.searchParams.get("state") || "");
  const expectedNonce = req.cookies.get(LARK_OAUTH_STATE_COOKIE)?.value;
  if (!state || !expectedNonce || state.nonce !== expectedNonce) {
    console.warn("[lark-auth] invalid oauth state");
    const response = NextResponse.json({ error: "Invalid OAuth state" }, { status: 400 });
    clearOAuthStateCookie(response);
    return response;
  }

  const apiBase = process.env.REMOTE_API_URL || "http://localhost:8080";
  console.info("[lark-auth] exchanging code with backend", {
    apiBase,
    redirectUri,
    codeSuffix: code.slice(-6),
  });
  const loginRes = await fetch(`${apiBase}/auth/lark`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ code, redirect_uri: redirectUri }),
    cache: "no-store",
  });

  if (!loginRes.ok) {
    const body = await loginRes.text();
    console.warn("[lark-auth] backend exchange failed", {
      status: loginRes.status,
      body: body.slice(0, 500),
    });
    return new NextResponse(body || "Lark login failed", {
      status: loginRes.status,
      headers: { "content-type": loginRes.headers.get("content-type") || "text/plain" },
    });
  }

  const appOrigin = new URL(redirectUri).origin;
  console.info("[lark-auth] backend exchange succeeded", { appOrigin });
  const response = NextResponse.redirect(buildPostAuthRedirect(appOrigin, state));
  clearOAuthStateCookie(response);
  for (const cookie of getSetCookies(loginRes.headers)) {
    response.headers.append("set-cookie", allowOAuthRedirectCookie(cookie));
  }
  return response;
}
