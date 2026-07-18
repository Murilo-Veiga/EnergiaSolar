import { createContext, useContext, type ReactNode } from "react";
import type { Plant } from "../lib/api";

interface PlantContextValue {
  plant: Plant;
}

const PlantContext = createContext<PlantContextValue | null>(null);

export function PlantProvider({ plant, children }: { plant: Plant; children: ReactNode }) {
  return <PlantContext.Provider value={{ plant }}>{children}</PlantContext.Provider>;
}

export function useActivePlant() {
  const ctx = useContext(PlantContext);
  if (!ctx) throw new Error("useActivePlant precisa estar dentro de um PlantProvider");
  return ctx.plant;
}
