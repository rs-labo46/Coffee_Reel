import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  ApiClientError,
  apiRequest,
} from "./client";
import {
  clearAccessToken,
  setAccessToken,
} from "../auth/tokenStore";

type FetchFunction = (
  input: RequestInfo | URL,
  init?: RequestInit,
) => Promise<Response>;

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: {
      "Content-Type": "application/json",
    },
  });
}

function noContentResponse(): Response {
  return new Response(null, { status: 204 });
}

function requestHeaders(call: [RequestInfo | URL, RequestInit?] | undefined): Headers {
  return new Headers(call?.[1]?.headers);
}

describe("API Client", () => {
  beforeEach(() => {
    clearAccessToken();
    document.cookie = "csrf_token=; Max-Age=0; Path=/";
  });

  it("認証RequestへAuthorization HeaderとCookie送信設定を付ける", async () => {
    setAccessToken("access-token");

    const fetchMock = vi.fn<FetchFunction>().mockResolvedValue(
      jsonResponse(200, {
        data: {
          id: 1,
        },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await apiRequest<{ data: { id: number } }>(
      "/me",
      { method: "GET" },
      { retryOnUnauthorized: false },
    );

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0]?.[0]).toBe("http://localhost:8080/me");
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({
      credentials: "include",
      method: "GET",
    });
    expect(requestHeaders(fetchMock.mock.calls[0]).get("Authorization")).toBe(
      "Bearer access-token",
    );
  });

  it("CSRF対象RequestへCookie値をHeaderとして設定する", async () => {
    document.cookie = "csrf_token=csrf-value; Path=/";

    const fetchMock = vi.fn<FetchFunction>().mockResolvedValue(noContentResponse());
    vi.stubGlobal("fetch", fetchMock);

    await apiRequest<void>(
      "/logout",
      { method: "POST" },
      {
        auth: false,
        csrf: true,
        retryOnUnauthorized: false,
      },
    );

    expect(requestHeaders(fetchMock.mock.calls[0]).get("X-CSRF-Token")).toBe(
      "csrf-value",
    );
  });

  it("401受信時にRefreshし、新しいAccess Tokenで元Requestを1回再送する", async () => {
    setAccessToken("old-token");
    document.cookie = "csrf_token=csrf-value; Path=/";

    const fetchMock = vi.fn<FetchFunction>(async (input, init) => {
      const url = String(input);
      const headers = new Headers(init?.headers);

      if (url.endsWith("/refresh")) {
        return jsonResponse(200, {
          data: {
            access_token: "new-token",
            token_type: "Bearer",
            expires_in: 900,
          },
        });
      }

      if (headers.get("Authorization") === "Bearer old-token") {
        return jsonResponse(401, {
          status: 401,
          code: "unauthorized",
          message: "認証情報が無効です",
          request_id: "req-old",
        });
      }

      return jsonResponse(200, {
        data: {
          ok: true,
        },
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    const response = await apiRequest<{ data: { ok: boolean } }>(
      "/protected",
      { method: "GET" },
    );

    expect(response.data.ok).toBe(true);
    expect(fetchMock).toHaveBeenCalledTimes(3);

    const refreshCall = fetchMock.mock.calls.find(
      ([input]) => String(input).endsWith("/refresh"),
    );
    expect(refreshCall).toBeDefined();
    expect(requestHeaders(refreshCall).get("X-CSRF-Token")).toBe(
      "csrf-value",
    );

    const retryCall = fetchMock.mock.calls[2];
    expect(requestHeaders(retryCall).get("Authorization")).toBe(
      "Bearer new-token",
    );
  });

  it("複数Requestが同時に401でもRefresh Requestを1回だけ実行する", async () => {
    setAccessToken("old-token");
    document.cookie = "csrf_token=csrf-value; Path=/";

    let refreshCount = 0;

    const fetchMock = vi.fn<FetchFunction>(async (input, init) => {
      const url = String(input);
      const headers = new Headers(init?.headers);

      if (url.endsWith("/refresh")) {
        refreshCount += 1;
        await Promise.resolve();

        return jsonResponse(200, {
          data: {
            access_token: "new-token",
            token_type: "Bearer",
            expires_in: 900,
          },
        });
      }

      if (headers.get("Authorization") === "Bearer old-token") {
        return jsonResponse(401, {
          status: 401,
          code: "unauthorized",
          message: "認証情報が無効です",
          request_id: "req-old",
        });
      }

      return jsonResponse(200, {
        data: {
          path: url,
        },
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    const [first, second] = await Promise.all([
      apiRequest<{ data: { path: string } }>("/first", { method: "GET" }),
      apiRequest<{ data: { path: string } }>("/second", { method: "GET" }),
    ]);

    expect(first.data.path).toContain("/first");
    expect(second.data.path).toContain("/second");
    expect(refreshCount).toBe(1);
  });

  it("Refresh後の再送も401なら再Refreshせず終了する", async () => {
    setAccessToken("old-token");
    document.cookie = "csrf_token=csrf-value; Path=/";

    const fetchMock = vi
      .fn<FetchFunction>()
      .mockResolvedValueOnce(
        jsonResponse(401, {
          status: 401,
          code: "unauthorized",
          message: "認証情報が無効です",
          request_id: "req-first",
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse(200, {
          data: {
            access_token: "new-token",
            token_type: "Bearer",
            expires_in: 900,
          },
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse(401, {
          status: 401,
          code: "unauthorized",
          message: "認証情報が無効です",
          request_id: "req-retry",
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      apiRequest("/protected", { method: "GET" }),
    ).rejects.toMatchObject({
      status: 401,
      code: "unauthorized",
      requestId: "req-retry",
    });

    expect(fetchMock).toHaveBeenCalledTimes(3);
  });

  it("Backend共通エラーをApiClientErrorへ変換する", async () => {
    const fetchMock = vi.fn<FetchFunction>().mockResolvedValue(
      jsonResponse(429, {
        status: 429,
        code: "rate_limit_exceeded",
        message: "リクエスト回数が上限を超えました",
        request_id: "req-rate-limit",
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    try {
      await apiRequest(
        "/login",
        { method: "POST" },
        {
          auth: false,
          retryOnUnauthorized: false,
        },
      );
      throw new Error("ApiClientErrorが必要です");
    } catch (error: unknown) {
      expect(error).toBeInstanceOf(ApiClientError);
      expect(error).toMatchObject({
        status: 429,
        code: "rate_limit_exceeded",
        message: "リクエスト回数が上限を超えました",
        requestId: "req-rate-limit",
      });
    }
  });
});
