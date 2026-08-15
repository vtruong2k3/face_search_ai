"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import * as api from "@/lib/api";

interface AuthContextValue {
  user: api.User | null;
  restoring: boolean;
  organizations: api.OrganizationMembership[];
  organizationsLoading: boolean;
  organizationsError: boolean;
  currentOrganization: api.OrganizationMembership | null;
  selectOrganization(organizationId: string): void;
  register(credentials: api.Credentials): Promise<void>;
  login(credentials: api.Credentials): Promise<void>;
  logout(): Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<api.User | null>(null);
  const [restoring, setRestoring] = useState(true);
  const [organizations, setOrganizations] = useState<api.OrganizationMembership[]>([]);
  const [organizationsLoading, setOrganizationsLoading] = useState(false);
  const [organizationsError, setOrganizationsError] = useState(false);
  const [currentOrganization, setCurrentOrganization] = useState<api.OrganizationMembership | null>(null);

  const clearOrganizations = useCallback(() => {
    setOrganizations([]);
    setCurrentOrganization(null);
    setOrganizationsError(false);
    setOrganizationsLoading(false);
  }, []);

  const loadOrganizations = useCallback(async () => {
    setOrganizationsLoading(true);
    setOrganizationsError(false);
    try {
      const memberships = await api.listOrganizations();
      setOrganizations(memberships);
      setCurrentOrganization((current) => memberships.find((membership) => membership.organizationId === current?.organizationId) ?? memberships[0] ?? null);
    } catch {
      setOrganizations([]);
      setCurrentOrganization(null);
      setOrganizationsError(true);
    } finally {
      setOrganizationsLoading(false);
    }
  }, []);

  useEffect(() => {
    api.refresh()
      .then(async (result) => { setUser(result.user); await loadOrganizations(); })
      .catch(() => { setUser(null); clearOrganizations(); })
      .finally(() => setRestoring(false));
  }, [clearOrganizations, loadOrganizations]);

  const establishSession = useCallback(async (result: api.AuthResponse) => {
    setUser(result.user);
    await loadOrganizations();
  }, [loadOrganizations]);
  const register = useCallback(async (credentials: api.Credentials) => establishSession(await api.register(credentials)), [establishSession]);
  const login = useCallback(async (credentials: api.Credentials) => establishSession(await api.login(credentials)), [establishSession]);
  const logout = useCallback(async () => {
    await api.logout();
    setUser(null);
    clearOrganizations();
  }, [clearOrganizations]);
  const selectOrganization = useCallback((organizationId: string) => {
    setCurrentOrganization(organizations.find((membership) => membership.organizationId === organizationId) ?? null);
  }, [organizations]);

  const value = useMemo(() => ({ user, restoring, organizations, organizationsLoading, organizationsError, currentOrganization, selectOrganization, register, login, logout }), [user, restoring, organizations, organizationsLoading, organizationsError, currentOrganization, selectOrganization, register, login, logout]);
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) throw new Error("useAuth must be used within AuthProvider");
  return value;
}
