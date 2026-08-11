type AccessTokenListener = (accessToken: string | null) => void;

let accessToken: string | null = null;
let csrfToken: string | null = null;

const listeners = new Set<AccessTokenListener>();

export function getAccessToken(): string | null {
  return accessToken;
}

export function setAccessToken(token: string): void {
  accessToken = token;
  notifyAccessToken();
}

export function clearAccessToken(): void {
  accessToken = null;
  notifyAccessToken();
}

export function getCSRFToken(): string | null {
  return csrfToken;
}

export function setCSRFToken(token: string): void {
  csrfToken = token;
}

export function clearCSRFToken(): void {
  csrfToken = null;
}

export function clearAuthTokens(): void {
  accessToken = null;
  csrfToken = null;
  notifyAccessToken();
}

export function subscribeAccessToken(
  listener: AccessTokenListener,
): () => void {
  listeners.add(listener);

  return () => {
    listeners.delete(listener);
  };
}

function notifyAccessToken(): void {
  for (const listener of listeners) {
    listener(accessToken);
  }
}
