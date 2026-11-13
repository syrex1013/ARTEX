"use client";

import { useEffect, useState } from "react";

import { http } from "@/lib/api";
import type { CurrentUser } from "@/lib/auth";

const FALLBACK: CurrentUser = { id: "", name: "Loading…", username: "", email: "", avatar: "", role: "loading" };
let currentRequest: Promise<CurrentUser> | undefined;
export function loadCurrentUser() {
  currentRequest ??= http<{
    id: number;
    username: string;
    display_name: string;
    admin: boolean;
    preferences?: { timezone?: string; density?: string };
  }>("/auth/me").then((u) => {
    if (u.preferences?.timezone) localStorage.setItem("artex_timezone", u.preferences.timezone);
    if (u.preferences?.density) document.documentElement.dataset.density = u.preferences.density;
    return {
      id: String(u.id),
      name: u.display_name,
      username: u.username,
      email: "",
      avatar: "",
      role: u.admin ? "admin" : "member",
    };
  });
  return currentRequest;
}
export function useCurrentUser(): CurrentUser {
  const [user, setUser] = useState(FALLBACK);
  useEffect(() => {
    let active = true;
    void loadCurrentUser()
      .then((u) => {
        if (active) setUser(u);
      })
      .catch(() => {
        currentRequest = undefined;
        if (active) setUser({ ...FALLBACK, name: "Account unavailable", role: "error" });
      });
    return () => {
      active = false;
    };
  }, []);
  return user;
}
