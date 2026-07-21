import {
  clearAccessToken,
  getAccessToken,
  setAccessToken,
} from "../auth/tokenStore";
import type { ApiErrorResponse, RefreshResponse } from "../types/user";

const rawApiURL = import.meta.env.VITE_API_URL;

if (typeof rawApiURL !== "string" || rawApiURL.trim() === "") {
  throw new Error("VITE_API_URL is required");
}

const apiURL = rawApiURL.replace(/\/+$/, "");
const refreshPath = "/refresh";
const csrfCookieName = "csrf_token";
const csrfHeaderName = "X-CSRF-Token";

let refreshPromise: Promise<string> | null = null;

type ApiRequestOptions = {
  auth?: boolean;
  retryOnUnauthorized?: boolean;
};

export class ApiClientError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId: string;

  constructor(status: number, code: string, message: string, requestId = "") {
    super(message);
    this.name = "ApiClientError";
    this.status = status;
    this.code = code;
    this.requestId = requestId;
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isApiErrorResponse(value: unknown): value is ApiErrorResponse {
  if (!isRecord(value)) {
    return false;
  }

  return (
    typeof value.status === "number" &&
    typeof value.code === "string" &&
    typeof value.message === "string" &&
    typeof value.request_id === "string"
  );
}

// unknown型の値が、Access Token再発行APIの正常Response形式かどうか
function isRefreshResponse(value: unknown): value is RefreshResponse {
  if (!isRecord(value) || !isRecord(value.data)) {
    return false;
  }

  return (
    typeof value.data.access_token === "string" &&
    value.data.access_token !== "" &&
    value.data.token_type === "Bearer" &&
    typeof value.data.expires_in === "number"
  );
}

//Response Bodyを文字列として読み取り、JSONへ変換
async function readJSON(response: Response): Promise<unknown> {
  const text = await response.text();

  if (text === "") {
    return null;
  }

  try {
    return JSON.parse(text) as unknown;
  } catch {
    throw new ApiClientError(
      response.status,
      "invalid_response",
      "サーバーから不正なレスポンスを受信しました",
    );
  }
}

// 共通Header、Cookie送信、Authorization Header、Response解析、Backend ErrorのApiClientError変換
async function sendRequest<T>(
  path: string,
  init: RequestInit,
  accessToken: string | null,
): Promise<T> {
  const headers = new Headers(init.headers);

  if (!headers.has("Accept")) {
    headers.set("Accept", "application/json");
  }

  if (accessToken !== null && !headers.has("Authorization")) {
    headers.set("Authorization", `Bearer ${accessToken}`);
  }

  let response: Response;

  try {
    response = await fetch(`${apiURL}${path}`, {
      ...init,
      credentials: "include",
      headers,
    });
  } catch {
    throw new ApiClientError(
      0,
      "network_error",
      "APIへ接続できません。通信状態を確認してください",
    );
  }

  const body = await readJSON(response);

  if (!response.ok) {
    if (isApiErrorResponse(body)) {
      throw new ApiClientError(
        body.status,
        body.code,
        body.message,
        body.request_id,
      );
    }

    throw new ApiClientError(
      response.status,
      "request_failed",
      "リクエストに失敗しました",
    );
  }

  if (body === null) {
    throw new ApiClientError(
      response.status,
      "invalid_response",
      "サーバーから必要なデータを受信できませんでした",
    );
  }

  return body as T;
}

// Backend APIを呼び出す共通関数。認証が必要なRequestへAccess Tokenを付け、401時はTokenを再発行して1回だけ再送する。
export async function apiRequest<T>(
  path: string,
  init: RequestInit,
  options: ApiRequestOptions = {},
): Promise<T> {
  const usesAuth = options.auth !== false;
  const tokenUsed = usesAuth ? getAccessToken() : null;

  try {
    return await sendRequest<T>(path, init, tokenUsed);
  } catch (error: unknown) {
    const canRetry =
      error instanceof ApiClientError &&
      error.status === 401 &&
      usesAuth &&
      options.retryOnUnauthorized !== false &&
      path !== refreshPath &&
      tokenUsed !== null;

    if (!canRetry) {
      throw error;
    }

    const currentToken = getAccessToken();

    if (currentToken !== null && currentToken !== tokenUsed) {
      return retryRequest<T>(path, init, currentToken);
    }

    const refreshedAccessToken = await refreshAccessToken();

    return retryRequest<T>(path, init, refreshedAccessToken);
  }
}

// Access Token再発行処理を開始する。既に再発行中の場合は、同じPromiseを返してRefresh Requestの重複を防ぐ。
export function refreshAccessToken(): Promise<string> {
  if (refreshPromise !== null) {
    return refreshPromise;
  }

  refreshPromise = requestNewAccessToken().finally(() => {
    refreshPromise = null;
  });

  return refreshPromise;
}

// CSRF CookieとRefresh Token Cookieを使用して、新しいAccess Tokenを取得する。成功時はToken Storeへ保存し、認証失敗時は保持中のAccess Tokenを削除する。
async function requestNewAccessToken(): Promise<string> {
  const csrfToken = readCookie(csrfCookieName);

  if (csrfToken === "") {
    clearAccessToken();
    throw new ApiClientError(
      403,
      "csrf_invalid",
      "CSRFトークンが見つかりません",
    );
  }

  try {
    const response = await sendRequest<RefreshResponse>(
      refreshPath,
      {
        method: "POST",
        headers: {
          [csrfHeaderName]: csrfToken,
        },
      },
      null,
    );

    if (!isRefreshResponse(response)) {
      throw new ApiClientError(
        200,
        "invalid_response",
        "Access Tokenの再発行結果が不正です",
      );
    }

    setAccessToken(response.data.access_token);

    return response.data.access_token;
  } catch (error: unknown) {
    if (
      error instanceof ApiClientError &&
      (error.status === 401 || error.status === 403)
    ) {
      clearAccessToken();
    }

    throw error;
  }
}

// 再発行したAccess Tokenを使用して、元のRequestを1回だけ再送する。再送後も401の場合はAccess Tokenを削除し、それ以上の再試行は行わない。
async function retryRequest<T>(
  path: string,
  init: RequestInit,
  accessToken: string,
): Promise<T> {
  try {
    return await sendRequest<T>(path, init, accessToken);
  } catch (error: unknown) {
    if (error instanceof ApiClientError && error.status === 401) {
      clearAccessToken();
    }

    throw error;
  }
}

// document.cookieから指定したCookieの値を取得する。HttpOnly CookieはJavaScriptから読めないため、CSRF Cookieの取得にだけ使用する。
function readCookie(name: string): string {
  if (typeof document === "undefined") {
    return "";
  }

  const cookieName = `${encodeURIComponent(name)}=`;
  const cookies = document.cookie.split(";");

  for (const cookie of cookies) {
    const value = cookie.trim();

    if (value.startsWith(cookieName)) {
      return decodeURIComponent(value.slice(cookieName.length));
    }
  }

  return "";
}
