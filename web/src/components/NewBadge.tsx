// Espelha NEW_FEATURES_SINCE/isFeatureNew/paintNewBadges do original — selo
// "novo" por 5 dias corridos a partir da data de lançamento da seção/aba.
const NEW_FEATURE_DAYS = 5;
const NEW_FEATURES_SINCE: Record<string, string> = {
  "nav-saude": "2026-07-16",
  "hist-streak": "2026-07-16",
  "hist-yield": "2026-07-16",
  "saude-reliability": "2026-07-16",
  "saude-contrib-range": "2026-07-16",
};

export function NewBadge({ featureKey }: { featureKey: string }) {
  const since = NEW_FEATURES_SINCE[featureKey];
  if (!since) return null;
  const ageDays = (Date.now() - new Date(`${since}T00:00:00`).getTime()) / 86400000;
  if (!(ageDays >= 0 && ageDays < NEW_FEATURE_DAYS)) return null;
  return <span className="new-badge">novo</span>;
}
