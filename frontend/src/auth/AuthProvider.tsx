import { useEffect, useRef, useState, type ReactNode } from "react";
import type { AuthState, LoginInput, User } from "../types/user";
import {
  fetchCSRFToken,
  getMe,
  login as requestLogin,
  logout as requestLogout,
} from "../api/user";
import {
  clearAuthTokens,
  setAccessToken,
  setCSRFToken,
  subscribeAccessToken,
} from "./tokenStore";
import { AuthContext, type AuthContextValue } from "./authContext";
import { refreshAccessToken } from "../api/client";

type RestoredSession = {
  user: User;
  accessToken: string;
};

const initialState: AuthState = {
  user: null,
  accessToken: null,
  isAuthenticated: false,
  isLoading: true,
};

type AuthProviderProps = { children: ReactNode };

export function AuthProvider({ children }: AuthProviderProps) {
  const [state, setState] = useState<AuthState>(initialState);
  const restorePromiseRef = useRef<Promise<RestoredSession> | null>(null);
  const authRevisionRef = useRef(0);

  useEffect(() => {
    return subscribeAccessToken((accessToken) => {
      setState((currentState) => {
        if (accessToken === null) {
          return {
            user: null,
            accessToken: null,
            isAuthenticated: false,
            isLoading: false,
          };
        }

        return {
          ...currentState,
          accessToken,
        };
      });
    });
  }, []);

  useEffect(() => {
    let isActive = true;
    const authRevision = authRevisionRef.current;

    if (restorePromiseRef.current === null) {
      restorePromiseRef.current = restoreSession();
    }

    restorePromiseRef.current
      .then((session) => {
        if (!isActive || authRevision !== authRevisionRef.current) {
          return;
        }

        setState({
          user: session.user,
          accessToken: session.accessToken,
          isAuthenticated: true,
          isLoading: false,
        });
      })
      .catch(() => {
        if (!isActive || authRevision !== authRevisionRef.current) {
          return;
        }

        clearAuthTokens();
        setState({
          user: null,
          accessToken: null,
          isAuthenticated: false,
          isLoading: false,
        });
      });

    return () => {
      isActive = false;
    };
  }, []);

  async function login(input: LoginInput): Promise<void> {
    const response = await requestLogin(input);

    authRevisionRef.current += 1;
    setCSRFToken(response.data.csrf_token);
    setAccessToken(response.data.access_token);
    setState({
      user: response.data.user,
      accessToken: response.data.access_token,
      isAuthenticated: true,
      isLoading: false,
    });
  }

  async function logout(): Promise<void> {
    authRevisionRef.current += 1;

    await requestLogout();

    clearAuthTokens();
    setState({
      user: null,
      accessToken: null,
      isAuthenticated: false,
      isLoading: false,
    });
  }

  const value: AuthContextValue = {
    ...state,
    login,
    logout,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

async function restoreSession(): Promise<RestoredSession> {
  const csrfResponse = await fetchCSRFToken();
  setCSRFToken(csrfResponse.data.csrf_token);

  const accessToken = await refreshAccessToken();
  const response = await getMe();

  return {
    user: response.data,
    accessToken,
  };
}
