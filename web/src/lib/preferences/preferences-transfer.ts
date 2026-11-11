"use client";

import { CHAT_SEND_MODE_KEY, type ChatSendMode, setChatSendMode } from "@/lib/chat-send-mode";
import { fontRegistry } from "@/lib/fonts/registry";
import {
  CONTENT_LAYOUT_VALUES,
  type ContentLayout,
  NAVBAR_STYLE_VALUES,
  type NavbarStyle,
  SIDEBAR_COLLAPSIBLE_VALUES,
  SIDEBAR_VARIANT_VALUES,
  type SidebarCollapsible,
  type SidebarVariant,
} from "@/lib/preferences/layout";
import {
  applyContentLayout,
  applyFont,
  applyNavbarStyle,
  applySidebarCollapsible,
  applySidebarVariant,
} from "@/lib/preferences/layout-utils";
import { persistPreference } from "@/lib/preferences/preferences-storage";
import { THEME_MODE_VALUES, THEME_PRESET_VALUES, type ThemeMode, type ThemePreset } from "@/lib/preferences/theme";
import { applyThemePreset } from "@/lib/preferences/theme-utils";
import type { PreferencesState } from "@/stores/preferences/preferences-store";

/**
 * Browser-only preferences (theme, layout, chat key binding) live in cookies / localStorage, so the server cannot
 * include them in a configuration bundle. The web client reads and applies them itself.
 */

export function collectPreferences(s: PreferencesState): Record<string, string> {
  return {
    theme_mode: s.themeMode,
    theme_preset: s.themePreset,
    font: s.font,
    content_layout: s.contentLayout,
    navbar_style: s.navbarStyle,
    sidebar_variant: s.sidebarVariant,
    sidebar_collapsible: s.sidebarCollapsible,
    [CHAT_SEND_MODE_KEY]: localStorage.getItem(CHAT_SEND_MODE_KEY) ?? "",
  };
}

const oneOf = <T extends string>(values: readonly T[], v: unknown): v is T =>
  typeof v === "string" && (values as readonly string[]).includes(v);

/** Applies every valid value of an imported `preferences` object; returns the keys that were applied. Unknown or invalid values are ignored. */
export function applyImportedPreferences(prefs: Record<string, unknown> | undefined, s: PreferencesState): string[] {
  const applied: string[] = [];
  if (!prefs) return applied;

  if (oneOf<ThemeMode>(THEME_MODE_VALUES, prefs.theme_mode)) {
    s.setThemeMode(prefs.theme_mode);
    void persistPreference("theme_mode", prefs.theme_mode);
    applied.push("theme_mode");
  }
  if (oneOf<ThemePreset>(THEME_PRESET_VALUES, prefs.theme_preset)) {
    applyThemePreset(prefs.theme_preset);
    s.setThemePreset(prefs.theme_preset);
    void persistPreference("theme_preset", prefs.theme_preset);
    applied.push("theme_preset");
  }
  if (typeof prefs.font === "string" && prefs.font in fontRegistry) {
    const font = prefs.font as keyof typeof fontRegistry;
    applyFont(font);
    s.setFont(font);
    void persistPreference("font", font);
    applied.push("font");
  }
  if (oneOf<ContentLayout>(CONTENT_LAYOUT_VALUES, prefs.content_layout)) {
    applyContentLayout(prefs.content_layout);
    s.setContentLayout(prefs.content_layout);
    void persistPreference("content_layout", prefs.content_layout);
    applied.push("content_layout");
  }
  if (oneOf<NavbarStyle>(NAVBAR_STYLE_VALUES, prefs.navbar_style)) {
    applyNavbarStyle(prefs.navbar_style);
    s.setNavbarStyle(prefs.navbar_style);
    void persistPreference("navbar_style", prefs.navbar_style);
    applied.push("navbar_style");
  }
  if (oneOf<SidebarVariant>(SIDEBAR_VARIANT_VALUES, prefs.sidebar_variant)) {
    applySidebarVariant(prefs.sidebar_variant);
    s.setSidebarVariant(prefs.sidebar_variant);
    void persistPreference("sidebar_variant", prefs.sidebar_variant);
    applied.push("sidebar_variant");
  }
  if (oneOf<SidebarCollapsible>(SIDEBAR_COLLAPSIBLE_VALUES, prefs.sidebar_collapsible)) {
    applySidebarCollapsible(prefs.sidebar_collapsible);
    s.setSidebarCollapsible(prefs.sidebar_collapsible);
    void persistPreference("sidebar_collapsible", prefs.sidebar_collapsible);
    applied.push("sidebar_collapsible");
  }
  if (oneOf<ChatSendMode>(["enter", "ctrl-enter"], prefs[CHAT_SEND_MODE_KEY])) {
    setChatSendMode(prefs[CHAT_SEND_MODE_KEY] as ChatSendMode);
    applied.push(CHAT_SEND_MODE_KEY);
  }
  return applied;
}
