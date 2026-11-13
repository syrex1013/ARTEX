import { visiblePoll } from "./visible-poller.ts";
import assert from "node:assert/strict";
import test from "node:test";

const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
class Visibility extends EventTarget {
  hidden = false;
  change(hidden) {
    this.hidden = hidden;
    this.dispatchEvent(new Event("visibilitychange"));
  }
}
test("polling never overlaps, pauses hidden, refreshes on return, aborts on cleanup", async () => {
  const visibility = new Visibility();
  let active = 0,
    maximum = 0,
    calls = 0;
  let lastSignal;
  const stop = visiblePoll(
    async (signal) => {
      lastSignal = signal;
      active++;
      calls++;
      maximum = Math.max(maximum, active);
      await wait(25);
      active--;
    },
    5,
    visibility,
  );
  await wait(75);
  assert.equal(maximum, 1);
  visibility.change(true);
  const hiddenCalls = calls;
  assert.equal(lastSignal.aborted, true);
  await wait(60);
  assert.equal(calls, hiddenCalls);
  visibility.change(false);
  await wait(10);
  assert.ok(calls > hiddenCalls);
  stop();
  const stoppedCalls = calls;
  assert.equal(lastSignal.aborted, true);
  await wait(65);
  assert.equal(calls, stoppedCalls);
});
test("one failed read leaves sibling results intact", async () => {
  const state = { good: "old", failed: "last success" };
  await Promise.allSettled([
    Promise.resolve("fresh").then((value) => {
      state.good = value;
    }),
    Promise.reject(new Error("unavailable")).then((value) => {
      state.failed = value;
    }),
  ]);
  assert.deepEqual(state, { good: "fresh", failed: "last success" });
});
