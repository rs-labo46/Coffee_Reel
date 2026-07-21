import { createContext, useContext, useState, type ReactNode } from "react";
import type { AuthState, LoginInput } from "../types/user";
import { login as requestLogin } from "../api/user";
type AuthContextValue = AuthState & {
  login: (input: LoginInput) => Promise<void>;
};

const initialState: AuthState = {
  user: null,
  accessToken: null,
  isAuthenticated: false,
  isLoading: false,
};

const AuthContext = createContext<AuthContextValue | null>(null);

type AuthProviderProps = { children: ReactNode };

export function AuthProvider({ children }: AuthProviderProps) {
  const [state, setState] = useState<AuthState>(initialState);

  async function login(input: LoginInput): Promise<void> {
    const response = await requestLogin(input);

    setState({
      user: response.data.user,
      accessToken: response.data.access_token,
      isAuthenticated: true,
      isLoading: false,
    });
  }
  const value: AuthContextValue = {
    ...state,
    login,
  };
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

//どのComponentからでもAuthContextの認証情報を安全に取得するための専用関数
export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);

  if (context === null) {
    throw new Error("useAuth must be used within AuthProvider");
  }

  return context;
}
