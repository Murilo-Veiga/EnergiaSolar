import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { api, ApiError, type Me, type Plant } from "../lib/api";

interface AuthContextValue {
  authenticated: boolean;
  loading: boolean;
  isAdmin: boolean;
  userId: string | null;
  plants: Plant[];
  refreshPlants: () => Promise<void>;
  login: (identifier: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

// Não existe endpoint "whoami" dedicado pra plants — descobrimos se a sessão
// (cookie httpOnly) ainda é válida chamando /api/plants: 200 = autenticado,
// 401 = não. is_admin vem de /api/me, chamado em paralelo.
export function AuthProvider({ children }: { children: ReactNode }) {
  const [authenticated, setAuthenticated] = useState(false);
  const [loading, setLoading] = useState(true);
  const [isAdmin, setIsAdmin] = useState(false);
  const [userId, setUserId] = useState<string | null>(null);
  const [plants, setPlants] = useState<Plant[]>([]);

  const refreshPlants = useCallback(async () => {
    try {
      const list = await api.get<Plant[]>("/api/plants");
      setPlants(list);
      setAuthenticated(true);
      try {
        const me = await api.get<Me>("/api/me");
        setIsAdmin(me.is_admin);
        setUserId(me.user_id);
      } catch {
        setIsAdmin(false);
        setUserId(null);
      }
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setAuthenticated(false);
        setPlants([]);
        setIsAdmin(false);
        setUserId(null);
      } else {
        throw err;
      }
    }
  }, []);

  useEffect(() => {
    refreshPlants().finally(() => setLoading(false));
  }, [refreshPlants]);

  const login = useCallback(
    async (identifier: string, password: string) => {
      // O backend aceita e-mail ou username no mesmo campo "email".
      await api.post("/api/auth/login", { email: identifier, password });
      await refreshPlants();
    },
    [refreshPlants],
  );

  const logout = useCallback(async () => {
    await api.post("/api/auth/logout");
    setAuthenticated(false);
    setPlants([]);
  }, []);

  return (
    <AuthContext.Provider value={{ authenticated, loading, isAdmin, userId, plants, refreshPlants, login, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth precisa estar dentro de um AuthProvider");
  return ctx;
}
