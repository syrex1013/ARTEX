import { dailyUsage, formatTokens, sumUsage, tokenBreakdown } from "./dashboard-usage.ts";
import assert from "node:assert/strict";
import test from "node:test";

test("compact token units remain readable through billions", () => {
  assert.equal(formatTokens(0), "0");
  assert.equal(formatTokens(999), "999");
  assert.equal(formatTokens(999_999), "1M");
  assert.equal(formatTokens(22_839_000_000), "22.8B");
});

test("cached input is counted once, including providers with invalid cache subsets", () => {
  assert.deepEqual(tokenBreakdown(100, 20, 80), { input: 20, output: 20, cacheRead: 80, total: 120, hitRate: 80 });
  assert.equal(tokenBreakdown(0, 0, 10).hitRate, 0);
  assert.equal(tokenBreakdown(10, 0, 20).cacheRead, 10);
});

test("daily range includes exactly N UTC days, fills gaps, excludes future rows and filters profiles", () => {
  const rows = [
    { date: "2026-12-31", profile: "name:deleted", input: 100, output: 20, cacheRead: 80 },
    { date: "2027-01-02", profile: "name:active", input: 40, output: 10, cacheRead: 10 },
    { date: "2026-12-30", profile: "name:active", input: 999, output: 0, cacheRead: 0 },
    { date: "2027-01-03", profile: "name:active", input: 999, output: 0, cacheRead: 0 },
  ];
  const now = new Date("2027-01-02T00:30:00Z");
  const all = dailyUsage(rows, 3, "all", now);
  assert.deepEqual(
    all.map((day) => day.date),
    ["2026-12-31", "2027-01-01", "2027-01-02"],
  );
  assert.equal(
    all.reduce((n, day) => n + day.input + day.output + day.cacheRead, 0),
    170,
  );
  assert.equal(all[1].input, 0);
  const selected = dailyUsage(rows, 3, "name:deleted", now);
  assert.equal(
    selected.reduce((n, day) => n + day.input + day.output + day.cacheRead, 0),
    120,
  );
  assert.deepEqual(sumUsage(rows, "name:deleted"), { input: 100, output: 20, cacheRead: 80 });
});
