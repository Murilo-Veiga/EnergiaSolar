// Porta de iconSvg() em templates/index.html (Python/vanilla JS) — mesmo
// sistema de ícones "badge/flat" (SVG dentro de um chip colorido).
import type { JSX } from "react";

const ICONS: Record<string, (bg: string) => JSX.Element> = {
  sun: () => (
    <>
      <circle cx="12" cy="12" r="4.3" fill="currentColor" />
      {[0, 45, 90, 135, 180, 225, 270, 315].map((a) => (
        <rect key={a} x="11.25" y="1.3" width="1.5" height="3.7" rx="0.75" fill="currentColor" transform={`rotate(${a} 12 12)`} />
      ))}
    </>
  ),
  layoutGrid: () => (
    <>
      <rect x="3" y="3" width="7.5" height="7.5" rx="1.5" fill="currentColor" />
      <rect x="13.5" y="3" width="7.5" height="7.5" rx="1.5" fill="currentColor" />
      <rect x="3" y="13.5" width="7.5" height="7.5" rx="1.5" fill="currentColor" />
      <rect x="13.5" y="13.5" width="7.5" height="7.5" rx="1.5" fill="currentColor" />
    </>
  ),
  trendingUp: () => (
    <>
      <polyline points="3,17 9,11 13,15 21,6" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" />
      <polygon points="21,6 14.5,6 21,12.5" fill="currentColor" />
    </>
  ),
  activity: () => (
    <polyline points="2,12 7,12 9,5 13,19 15,12 22,12" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
  ),
  wallet: (bg) => (
    <>
      <rect x="3" y="6" width="18" height="13" rx="2.2" fill="currentColor" />
      <circle cx="17" cy="14.3" r="1.5" fill={bg} />
    </>
  ),
  star: () => <path d="M12 3.3 L14.6 8.9 L20.7 9.7 L16.3 13.8 L17.5 19.9 L12 16.8 L6.5 19.9 L7.7 13.8 L3.3 9.7 L9.4 8.9 Z" fill="currentColor" />,
  mapPin: (bg) => (
    <>
      <path d="M12 21.5s7-6.4 7-11.7a7 7 0 1 0-14 0c0 5.3 7 11.7 7 11.7z" fill="currentColor" />
      <circle cx="12" cy="9.8" r="2.3" fill={bg} />
    </>
  ),
  shuffle: () => (
    <>
      <polyline points="17,3 21,7 17,11" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M3 7h9.5a4 4 0 0 1 3.5 2" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      <polyline points="17,13 21,17 17,21" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M3 17h9.5a4 4 0 0 0 3.5-2" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </>
  ),
  alertTriangle: (bg) => (
    <>
      <path d="M12 3.2 L22 20.8 L2 20.8 Z" fill="currentColor" />
      <line x1="12" y1="9.3" x2="12" y2="14" stroke={bg} strokeWidth="1.8" strokeLinecap="round" />
      <line x1="12" y1="17.2" x2="12.01" y2="17.2" stroke={bg} strokeWidth="1.8" strokeLinecap="round" />
    </>
  ),
  thermometer: () => <path d="M12 14.5V4.3a2.1 2.1 0 0 0-4.2 0v10.2a4.1 4.1 0 1 0 4.2 0Z" fill="currentColor" />,
  flag: () => (
    <>
      <path d="M5 3v18" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      <path d="M5 4.2h11.5l-2.7 4.3l2.7 4.3H5Z" fill="currentColor" />
    </>
  ),
  trendingDown: () => (
    <>
      <polyline points="3,7 9,13 13,9 21,18" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" />
      <polygon points="21,18 21,11.5 14.5,18" fill="currentColor" />
    </>
  ),
  plug: () => (
    <>
      <path d="M9 2.5v4.2" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      <path d="M15 2.5v4.2" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      <path d="M7 7.7h10v4a5 5 0 0 1-10 0v-4Z" fill="currentColor" />
      <path d="M12 16.7v5" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </>
  ),
  target: () => (
    <>
      <circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" strokeWidth="2" />
      <circle cx="12" cy="12" r="5" fill="none" stroke="currentColor" strokeWidth="2" />
      <circle cx="12" cy="12" r="1.6" fill="currentColor" />
    </>
  ),
  shield: () => <path d="M12 2.5 L19.5 5.5 V11.5 C19.5 16.5 16.3 20 12 21.5 C7.7 20 4.5 16.5 4.5 11.5 V5.5 Z" fill="currentColor" />,
  cloud: () => <path d="M7.5 18.5a4.3 4.3 0 0 1-.4-8.58 5.4 5.4 0 0 1 10.4-1.9 3.9 3.9 0 0 1-.7 10.48Z" fill="currentColor" />,
  drizzle: () => (
    <>
      <path d="M7.5 14.5a4.3 4.3 0 0 1-.4-8.58 5.4 5.4 0 0 1 10.4-1.9 3.9 3.9 0 0 1-.7 10.48Z" fill="currentColor" />
      <line x1="8" y1="17.5" x2="7" y2="21" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
      <line x1="12" y1="17.5" x2="11" y2="21" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
      <line x1="16" y1="17.5" x2="15" y2="21" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
    </>
  ),
  user: () => (
    <>
      <circle cx="12" cy="8" r="3.6" fill="currentColor" />
      <path d="M4.5 20.5c0-4.1 3.4-6.8 7.5-6.8s7.5 2.7 7.5 6.8" fill="currentColor" />
    </>
  ),
  logout: () => (
    <>
      <path
        d="M10.5 3.5H6.2A1.7 1.7 0 0 0 4.5 5.2v13.6a1.7 1.7 0 0 0 1.7 1.7h4.3"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path d="M15.5 8 20 12 15.5 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      <line x1="20" y1="12" x2="9.5" y2="12" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </>
  ),
  settings: () => (
    <>
      <circle cx="12" cy="12" r="3.2" fill="none" stroke="currentColor" strokeWidth="2" />
      <path
        d="M12 2.5v2.4M12 19.1v2.4M21.5 12h-2.4M4.9 12H2.5M18.4 5.6l-1.7 1.7M7.3 16.7l-1.7 1.7M18.4 18.4l-1.7-1.7M7.3 7.3 5.6 5.6"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
      />
    </>
  ),
  menu: () => (
    <>
      <rect x="3" y="5.4" width="18" height="2.3" rx="1.15" fill="currentColor" />
      <rect x="3" y="10.85" width="18" height="2.3" rx="1.15" fill="currentColor" />
      <rect x="3" y="16.3" width="18" height="2.3" rx="1.15" fill="currentColor" />
    </>
  ),
  bell: () => (
    <>
      <path
        d="M12 2.5a5.6 5.6 0 0 0-5.6 5.6v3.1c0 1-.4 1.9-1.1 2.6l-1 1a1 1 0 0 0 .7 1.7h14a1 1 0 0 0 .7-1.7l-1-1a3.6 3.6 0 0 1-1.1-2.6V8.1A5.6 5.6 0 0 0 12 2.5Z"
        fill="currentColor"
      />
      <path d="M9.5 19.5a2.5 2.5 0 0 0 5 0" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
    </>
  ),
};

export function iconBody(name: string, bg: string): JSX.Element | null {
  const render = ICONS[name];
  return render ? render(bg) : null;
}

// Espelha ICON_BG em templates/index.html EXATAMENTE — precisa ser a MESMA
// cor translúcida de fundo do badge (não a cor sólida do traço), porque
// alguns ícones (wallet, mapPin, alertTriangle) têm um "furo" interno que
// só fica invisível se pintado com o fundo real do chip ao redor.
const COLOR_BG: Record<string, string> = {
  blue: "rgba(57,135,229,0.14)",
  aqua: "rgba(25,158,112,0.14)",
  green: "rgba(12,163,12,0.13)",
  gold: "rgba(250,178,25,0.14)",
  red: "rgba(208,59,59,0.16)",
};

export type IconColor = "blue" | "aqua" | "green" | "gold" | "red";

export function IconBadge({
  name,
  color = "blue",
  size = "nav",
}: {
  name: string;
  color?: IconColor;
  size?: "nav" | "card" | "alert" | "fc" | "lg";
}) {
  const bg = COLOR_BG[color];
  // Espelha paintIconBadges(): size-card renderiza o SVG em 18px, size-lg
  // (logo de telas de auth) em 24px, todo o resto (nav/alert/fc) em 15px —
  // só o viewBox interno continua 24x24.
  const pixelSize = size === "lg" ? 24 : size === "card" ? 18 : 15;
  return (
    <span className={`icon-badge ${color} size-${size}`}>
      <svg width={pixelSize} height={pixelSize} viewBox="0 0 24 24">
        {iconBody(name, bg)}
      </svg>
    </span>
  );
}
