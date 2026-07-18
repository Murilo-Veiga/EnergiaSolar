import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { api, ApiError, type Plant } from "../lib/api";

interface AuthContextValue {
  authenticated: boolean;
  loading: boolean;
  plants: Plant[];
  refreshPlants: () => Promise<void>;
  login: (email: string, password: string) => Promise<void>;
  signup: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

// Não existe endpoint "whoami" — descobrimos se a sessão (cookie httpOnly)
// ainda é válida chamando /api/plants: 200 = autenticado, 401 = não.
export function AuthProvider({ children }: { children: ReactNode }) {
  const [authenticated, setAuthenticated] = useState(false);
  const [loading, setLoading] = useState(true);
  const [plants, setPlants] = useState<Plant[]>([]);

  const refreshPlants = useCallback(async () => {
    try {
      const list = await api.get<Plant[]>("/api/plants");
      setPlants(list);
      setAuthenticated(true);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setAuthenticated(false);
        setPlants([]);
      } else {
        throw err;
      }
    }
  }, []);

  useEffect(() => {
    refreshPlants().finally(() => setLoading(false));
  }, [refreshPlants]);

  const login = useCallback(
    async (email: string, password: string) => {
      await api.post("/api/auth/login", { email, password });
      await refreshPlants();
    },
    [refreshPlants],
  );

  const signup = useCallback(
    async (email: string, password: string) => {
      await api.post("/api/auth/signup", { email, password });
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
    <AuthContext.Provider value={{ authenticated, loading, plants, refreshPlants, login, signup, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth precisa estar dentro de um AuthProvider");
  return ctx;
}
