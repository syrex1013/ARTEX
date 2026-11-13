export interface UsageRow {
  profile: string;
  input: number;
  output: number;
  cacheRead: number;
}

export interface DailyUsageRow extends UsageRow {
  date: string;
}

export function formatTokens(value: number): string {
  return new Intl.NumberFormat("en", { notation: "compact", maximumFractionDigits: 1 }).format(value);
}

export function tokenBreakdown(input: number, output: number, cacheRead: number) {
  const cached = Math.min(Math.max(0, cacheRead), Math.max(0, input));
  return {
    input: Math.max(0, input) - cached,
    output: Math.max(0, output),
    cacheRead: cached,
    total: Math.max(0, input) + Math.max(0, output),
    hitRate: input > 0 ? Math.round((cached / input) * 100) : 0,
  };
}

export function sumUsage(rows: UsageRow[], profile = "all") {
  return rows.reduce(
    (total, row) => {
      if (profile !== "all" && row.profile !== profile) return total;
      total.input += row.input;
      total.output += row.output;
      total.cacheRead += row.cacheRead;
      return total;
    },
    { input: 0, output: 0, cacheRead: 0 },
  );
}

export function dailyUsage(rows: DailyUsageRow[], days: number, profile = "all", now = new Date()) {
  const end = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()));
  const start = new Date(end);
  start.setUTCDate(start.getUTCDate() - days + 1);
  const buckets = new Map<string, { date: string; input: number; output: number; cacheRead: number }>();
  for (const day = new Date(start); day <= end; day.setUTCDate(day.getUTCDate() + 1)) {
    const date = day.toISOString().slice(0, 10);
    buckets.set(date, { date, input: 0, output: 0, cacheRead: 0 });
  }
  for (const row of rows) {
    if (profile !== "all" && row.profile !== profile) continue;
    const bucket = buckets.get(row.date);
    if (!bucket) continue;
    // Input includes cache reads; stacked chart segments must not overlap.
    const parts = tokenBreakdown(row.input, row.output, row.cacheRead);
    bucket.input += parts.input;
    bucket.output += parts.output;
    bucket.cacheRead += parts.cacheRead;
  }
  return [...buckets.values()];
}
