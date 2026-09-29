/**
 * A small in-memory, fixed-window rate limiter for the explorer's public route.
 *
 * The browser-facing `POST /api/graphql` is unauthenticated and read-only, and
 * every accepted request fans out to one indexer round-trip. This caps how many
 * requests a single client can drive through it in a window, a cheap-in guard
 * against the amplify-out cost (ADR-046).
 *
 * The window is per key (the caller supplies one, typically the client IP) and
 * the state is a process-local Map: it bounds a single instance and resets when
 * the process restarts. A deployment fronted by more than one instance needs a
 * shared store (Redis or an edge limiter) for a global cap; that is recorded in
 * ADR-046 as the production follow-up. This is the first line, not the last.
 */

/** The outcome of one rate-limit check. */
export interface RateLimitResult {
  /** Whether the request is under the limit and may proceed. */
  allowed: boolean;
  /** Requests still available in the current window (never negative). */
  remaining: number;
  /** Epoch milliseconds when the current window resets. */
  resetAt: number;
}

/** A keyed fixed-window limiter. */
export interface RateLimiter {
  check(key: string): RateLimitResult;
}

export interface RateLimiterOptions {
  /** Maximum requests allowed per key within one window. */
  limit: number;
  /** Window length in milliseconds. */
  windowMs: number;
  /** Injectable clock for tests; defaults to `Date.now`. */
  now?: () => number;
}

interface WindowState {
  count: number;
  windowStart: number;
}

/**
 * Create a fixed-window rate limiter.
 *
 * Each key gets a counter that resets when its window elapses. Stale entries are
 * swept lazily, once per window, so the Map does not grow without bound under a
 * churn of distinct keys.
 */
export function createRateLimiter(options: RateLimiterOptions): RateLimiter {
  const { limit, windowMs } = options;
  const now = options.now ?? Date.now;
  const windows = new Map<string, WindowState>();
  let lastSweep = now();

  function sweep(currentTime: number): void {
    if (currentTime - lastSweep < windowMs) return;
    for (const [key, state] of windows) {
      if (currentTime - state.windowStart >= windowMs) windows.delete(key);
    }
    lastSweep = currentTime;
  }

  return {
    check(key: string): RateLimitResult {
      const currentTime = now();
      sweep(currentTime);

      const existing = windows.get(key);
      const state =
        existing && currentTime - existing.windowStart < windowMs
          ? existing
          : { count: 0, windowStart: currentTime };

      state.count += 1;
      windows.set(key, state);

      const resetAt = state.windowStart + windowMs;
      const allowed = state.count <= limit;
      const remaining = allowed ? limit - state.count : 0;
      return { allowed, remaining, resetAt };
    },
  };
}
