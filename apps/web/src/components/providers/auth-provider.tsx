"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import * as api from "@/lib/api";

interface AuthContextValue {
  user: api.User | null;
  restoring: boolean;
  register(credentials: api.Credentials): Promise<void>;
  login(credentials: api.Credentials): Promise<void>;
  logout(): Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<api.User | null>(null);
  const [restoring, setRestoring] = useState(true);

  useEffect(() => {
    api.refresh().then((result) => setUser(result.user)).catch(() => setUser(null)).finally(() => setRestoring(false));
  }, []);

  const register = useCallback(async (credentials: api.Credentials) => {
    const result = await api.register(credentials);
    setUser(result.user);
  }, []);
  const login = useCallback(async (credentials: api.Credentials) => {
    const result = await api.login(credentials);
    setUser(result.user);
  }, []);
  const logout = useCallback(async () => {
    await api.logout();
    setUser(null);
  }, []);

  const value = useMemo(() => ({ user, restoring, register, login, logout }), [user, restoring, register, login, logout]);
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) throw new Error("useAuth must be used within AuthProvider");
  return value;
}
