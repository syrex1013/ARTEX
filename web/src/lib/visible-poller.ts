// Schedule after completion: a slow request cannot overlap the next poll.
export function visiblePoll(
  load: (signal: AbortSignal) => Promise<void>,
  delay: number,
  visibility: Pick<Document, "hidden" | "addEventListener" | "removeEventListener"> = document,
) {
  let stopped = false;
  let running = false;
  let refresh = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let controller: AbortController | undefined;
  const run = async () => {
    if (stopped || running || visibility.hidden) return;
    running = true;
    controller = new AbortController();
    try {
      await load(controller.signal);
    } catch (error) {
      if (!controller.signal.aborted) console.error("Polling failed", error);
    } finally {
      running = false;
      if (!stopped && !visibility.hidden) {
        const wait = refresh ? 0 : delay;
        refresh = false;
        timer = setTimeout(() => void run(), wait);
      }
    }
  };
  const changed = () => {
    clearTimeout(timer);
    if (visibility.hidden) controller?.abort();
    else {
      refresh = running;
      void run();
    }
  };
  visibility.addEventListener("visibilitychange", changed);
  void run();
  return () => {
    stopped = true;
    clearTimeout(timer);
    controller?.abort();
    visibility.removeEventListener("visibilitychange", changed);
  };
}
