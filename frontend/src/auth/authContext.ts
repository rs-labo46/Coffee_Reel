import { createContext } from "react";

import type { AuthState, LoginInput } from "../types/user";

// AuthContextを利用するComponentへ公開する値の型。
// AuthStateに加えて、ログイン処理を実行するlogin関数を持つ。
export type AuthContextValue = AuthState & {
  login: (input: LoginInput) => Promise<void>;
};

// 認証状態と認証操作をReact全体で共有するContext。
// Providerの外側で誤って使用したことを判定できるよう、初期値はnullにする。
export const AuthContext = createContext<AuthContextValue | null>(null);
