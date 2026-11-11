export const THEME_MODE_OPTIONS = [
  { label: "Light", value: "light" },
  { label: "Dark", value: "dark" },
  { label: "System", value: "system" },
] as const;

export const THEME_MODE_VALUES = THEME_MODE_OPTIONS.map((o) => o.value);
export type ThemeMode = (typeof THEME_MODE_VALUES)[number];
export type ResolvedThemeMode = "light" | "dark";

// --- generated:themePresets:start ---

export const THEME_PRESET_OPTIONS = [
  {
    label: "Default",
    value: "default",
    primary: {
      light: "oklch(0.205 0 0)",
      dark: "oklch(0.922 0 0)",
    },
  },
  {
    label: "Brutalist",
    value: "brutalist",
    primary: {
      light: "oklch(0.6489 0.237 26.9728)",
      dark: "oklch(0.7044 0.1872 23.1858)",
    },
  },
  {
    label: "Catppuccin",
    value: "catppuccin",
    primary: {
      light: "oklch(0.5547 0.2503 297.02)",
      dark: "oklch(0.7871 0.1187 304.77)",
    },
  },
  {
    label: "Dracula",
    value: "dracula",
    primary: {
      light: "oklch(0.5091 0.1878 287.15)",
      dark: "oklch(0.742 0.1485 301.88)",
    },
  },
  {
    label: "Gruvbox",
    value: "gruvbox",
    primary: {
      light: "oklch(0.6176 0.1277 70.67)",
      dark: "oklch(0.8325 0.1595 82.99)",
    },
  },
  {
    label: "Monokai",
    value: "monokai",
    primary: {
      light: "oklch(0.5767 0.1549 129.85)",
      dark: "oklch(0.8414 0.2044 127.29)",
    },
  },
  {
    label: "Nord",
    value: "nord",
    primary: {
      light: "oklch(0.5944 0.0772 254.03)",
      dark: "oklch(0.7746 0.0622 217.47)",
    },
  },
  {
    label: "One Dark",
    value: "one-dark",
    primary: {
      light: "oklch(0.6017 0.193 263.25)",
      dark: "oklch(0.7304 0.1213 245.3)",
    },
  },
  {
    label: "Rosé Pine",
    value: "rose-pine",
    primary: {
      light: "oklch(0.6171 0.0738 305.7)",
      dark: "oklch(0.776 0.0945 304.99)",
    },
  },
  {
    label: "Soft Pop",
    value: "soft-pop",
    primary: {
      light: "oklch(0.5106 0.2301 276.9656)",
      dark: "oklch(0.6801 0.1583 276.9349)",
    },
  },
  {
    label: "Solarized",
    value: "solarized",
    primary: {
      light: "oklch(0.6149 0.1394 244.93)",
      dark: "oklch(0.6149 0.1394 244.93)",
    },
  },
  {
    label: "Tangerine",
    value: "tangerine",
    primary: {
      light: "oklch(0.64 0.17 36.44)",
      dark: "oklch(0.64 0.17 36.44)",
    },
  },
  {
    label: "Tokyo Night",
    value: "tokyo-night",
    primary: {
      light: "oklch(0.5999 0.1804 257.53)",
      dark: "oklch(0.719 0.1322 264.2)",
    },
  },
] as const;

export const THEME_PRESET_VALUES = THEME_PRESET_OPTIONS.map((p) => p.value);

export type ThemePreset = (typeof THEME_PRESET_OPTIONS)[number]["value"];

// --- generated:themePresets:end ---
