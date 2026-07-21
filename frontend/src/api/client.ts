import type { ApiErrorResponse } from "../types/user";

const rawApiURL = import.meta.env.VITE_API_URL;

if (typeof rawApiURL !== "string" || rawApiURL.trim() === "") {
  throw new Error("VITE_API_URL is required");
}

const apiURL = rawApiURL.replace(/\/+$/, "");

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

export async function apiRequest<T>(path: string, init: RequestInit): Promise<T> {
  let response: Response;

  const headers = new Headers(init.headers);
  if (!headers.has("Accept")) {
    headers.set("Accept", "application/json");
  }

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
