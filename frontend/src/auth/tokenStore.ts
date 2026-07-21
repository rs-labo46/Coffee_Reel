// Access Tokenが変更されたときに呼び出すListenerの型。
// 現在のAccess Token、または未認証のnullを受け取る。
type AccessTokenListener = (accessToken: string | null) => void;

// Access Tokenをブラウザの永続Storageへ保存せず、メモリ上だけで保持する。
let accessToken: string | null = null;

// Access Tokenの変更を監視しているListenerを重複なく保持する。
const listeners = new Set<AccessTokenListener>();

// 現在メモリ上に保持しているAccess Tokenを取得する。
export function getAccessToken(): string | null {
  return accessToken;
}

// 新しいAccess Tokenをメモリへ保存し、登録済みListenerへ変更を通知する。
export function setAccessToken(nextAccessToken: string): void {
  accessToken = nextAccessToken;
  notifyListeners();
}

// Access Tokenをメモリから削除し、未認証状態になったことをListenerへ通知する。
export function clearAccessToken(): void {
  if (accessToken === null) {
    return;
  }

  accessToken = null;
  notifyListeners();
}

// Access Tokenの変更を監視するListenerを登録する。
// 戻り値の関数を実行すると、そのListenerの登録を解除できる。
export function subscribeAccessToken(
  listener: AccessTokenListener,
): () => void {
  listeners.add(listener);

  return () => {
    listeners.delete(listener);
  };
}

// 登録済みの全Listenerへ、現在のAccess Tokenを通知する。
function notifyListeners(): void {
  for (const listener of listeners) {
    listener(accessToken);
  }
}
