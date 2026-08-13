import {
  clearAuthTokens,
  getAccessToken,
  getCSRFToken,
  setAccessToken,
  setCSRFToken,
} from "../auth/tokenStore";
import type { ApiErrorResponse, RefreshResponse } from "../types/user";

const rawApiURL = import.meta.env.VITE_API_URL;

if (typeof rawApiURL !== "string" || rawApiURL.trim() === "") {
  throw new Error("VITE_API_URL is required");
}

const apiURL = rawApiURL.replace(/\/+$/, "");
const refreshPath = "/refresh";
const csrfHeaderName = "X-CSRF-Token";

let refreshPromise: Promise<RefreshResponse["data"]> | null = null;

type ApiRequestOptions = {
  auth?: boolean;
  csrf?: boolean;
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

function isRefreshResponse(value: unknown): value is RefreshResponse {
  if (!isRecord(value) || !isRecord(value.data) || !isRecord(value.data.user)) {
    return false;
  }

  const user = value.data.user;

  return (
    typeof value.data.access_token === "string" &&
    value.data.access_token !== "" &&
    value.data.token_type === "Bearer" &&
    typeof value.data.expires_in === "number" &&
    typeof value.data.csrf_token === "string" &&
    value.data.csrf_token !== "" &&
    typeof user.id === "number" &&
    user.id > 0 &&
    typeof user.name === "string" &&
    typeof user.email === "string" &&
    (user.role === "user" || user.role === "admin") &&
    (user.status === "active" || user.status === "suspended")
  );
}

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

// VITE_API_URLで指定したBackendへ直接Requestを送信する。
// Cross-SiteのRefresh Token Cookieを送信するためcredentialsをincludeに固定する。
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

  if (response.status === 204) {
    return undefined as T;
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

// Backend API共通処理。
// 認証が必要なRequestにはAccess Tokenを設定し、401時のみRefresh後に1回再送する。
export async function apiRequest<T>(
  path: string,
  init: RequestInit,
  options: ApiRequestOptions = {},
): Promise<T> {
  const usesAuth = options.auth !== false;
  const tokenUsed = usesAuth ? getAccessToken() : null;

  const requestInit = options.csrf === true ? addCSRFHeader(init) : init;

  try {
    return await sendRequest<T>(path, requestInit, tokenUsed);
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
      return retryRequest<T>(path, requestInit, currentToken);
    }

    const refreshedAccessToken = await refreshAccessToken();

    return retryRequest<T>(path, requestInit, refreshedAccessToken);
  }
}

// 同時に複数Requestが401になってもRefresh APIを1回だけ実行する。
export async function refreshAccessToken(): Promise<string> {
  const session = await refreshSession();

  return session.access_token;
}

// ページ再読み込み時はRefresh ResponseのUserも利用し、追加のGET /meを不要にする。
export function refreshSession(): Promise<RefreshResponse["data"]> {
  if (refreshPromise !== null) {
    return refreshPromise;
  }

  refreshPromise = requestNewSession().finally(() => {
    refreshPromise = null;
  });

  return refreshPromise;
}

// メモリ上のCSRF TokenとHttpOnly Refresh Token Cookieを利用して認証Sessionを再発行する。
async function requestNewSession(): Promise<RefreshResponse["data"]> {
  if (getCSRFToken() === null) {
    clearAuthTokens();

    throw new ApiClientError(
      403,
      "csrf_invalid",
      "CSRFトークンが見つかりません",
    );
  }

  try {
    const response = await sendRequest<RefreshResponse>(
      refreshPath,
      addCSRFHeader({
        method: "POST",
      }),
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
    setCSRFToken(response.data.csrf_token);

    return response.data;
  } catch (error: unknown) {
    if (
      error instanceof ApiClientError &&
      (error.status === 401 || error.status === 403)
    ) {
      clearAuthTokens();
    }

    throw error;
  }
}

// Refresh後のAccess Tokenで元Requestを1回だけ再送する。
async function retryRequest<T>(
  path: string,
  init: RequestInit,
  accessToken: string,
): Promise<T> {
  try {
    return await sendRequest<T>(path, init, accessToken);
  } catch (error: unknown) {
    if (error instanceof ApiClientError && error.status === 401) {
      clearAuthTokens();
    }

    throw error;
  }
}

// Backend Responseから取得してメモリ保持しているCSRF TokenをHeaderへ設定する。
function addCSRFHeader(init: RequestInit): RequestInit {
  const csrfToken = getCSRFToken();

  if (csrfToken === null) {
    throw new ApiClientError(
      403,
      "csrf_invalid",
      "CSRFトークンが見つかりません",
    );
  }

  const headers = new Headers(init.headers);

  headers.set(csrfHeaderName, csrfToken);

  return {
    ...init,
    headers,
  };
}
