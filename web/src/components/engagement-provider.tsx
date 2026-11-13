"use client";

import { createContext, type ReactNode, useContext, useEffect, useState } from "react";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { http } from "@/lib/api";

export interface Engagement {
  id: number;
  name: string;
  description: string;
  role: "manager" | "operator" | "viewer";
  status: string;
  // The seeded default engagement for unassigned tasks and chats (admin-only).
  legacy?: boolean;
  starts_on: string | null;
  ends_on: string | null;
  settings: {
    reporting_metadata?: Record<string, string>;
    default_report_model?: number;
    notifications?: boolean;
    retention_days?: number;
  };
}
const Context = createContext<{
  engagements: Engagement[];
  engagement?: Engagement;
  select: (id: number) => void;
  reload: () => void;
  error: string;
  loading: boolean;
} | null>(null);
export function EngagementProvider({ children }: { children: ReactNode }) {
  const [engagements, setEngagements] = useState<Engagement[]>([]);
  const [selected, setSelected] = useState(0);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [revision, setRevision] = useState(0);
  // biome-ignore lint/correctness/useExhaustiveDependencies: revision refreshes persisted data after successful mutations.
  useEffect(() => {
    const controller = new AbortController();
    void http<Engagement[]>("/collab/engagements?limit=200", { signal: controller.signal })
      .then((list) => {
        setEngagements(list);
        setError("");
        setSelected((current) =>
          list.some((e) => e.id === current)
            ? current
            : Number(localStorage.getItem("artex_engagement")) || list[0]?.id || 0,
        );
      })
      .catch((e: Error) => {
        if (!controller.signal.aborted) setError(e.message);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [revision]);
  const select = (id: number) => {
    setSelected(id);
    localStorage.setItem("artex_engagement", String(id));
  };
  const engagement = engagements.find((e) => e.id === selected) ?? engagements[0];
  useEffect(() => {
    if (engagement) localStorage.setItem("artex_engagement", String(engagement.id));
  }, [engagement]);
  return (
    <Context.Provider
      value={{ engagements, engagement, select, reload: () => setRevision((r) => r + 1), error, loading }}
    >
      {children}
    </Context.Provider>
  );
}
export function useEngagement() {
  const value = useContext(Context);
  if (!value) throw new Error("EngagementProvider required");
  return value;
}
export function EngagementSelector() {
  const { engagements, engagement, select, loading, error } = useEngagement();
  if (!engagements.length) {
    let text = "No engagements";
    if (error) text = "Engagements unavailable";
    else if (loading) text = "Loading engagements…";
    return <span className="px-2 text-muted-foreground text-sm">{text}</span>;
  }
  return (
    <Select value={String(engagement?.id ?? "")} onValueChange={(value) => select(Number(value))}>
      <SelectTrigger size="sm" className="max-w-56" aria-label="Current engagement">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {engagements.map((e) => (
          <SelectItem key={e.id} value={String(e.id)}>
            {e.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
