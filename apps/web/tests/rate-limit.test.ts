import { describe, expect, it } from 'vitest';

import { createRateLimiter } from '@/lib/http/rate-limit';

describe('createRateLimiter', () => {
  it('allows requests up to the limit and blocks the next one', () => {
    const limiter = createRateLimiter({ limit: 3, windowMs: 1000, now: () => 0 });
    expect(limiter.check('a').allowed).toBe(true);
    expect(limiter.check('a').allowed).toBe(true);
    const third = limiter.check('a');
    expect(third.allowed).toBe(true);
    expect(third.remaining).toBe(0);
    expect(limiter.check('a').allowed).toBe(false);
  });

  it('tracks each key independently', () => {
    const limiter = createRateLimiter({ limit: 1, windowMs: 1000, now: () => 0 });
    expect(limiter.check('a').allowed).toBe(true);
    expect(limiter.check('b').allowed).toBe(true);
    expect(limiter.check('a').allowed).toBe(false);
  });

  it('resets the count once the window elapses', () => {
    let clock = 0;
    const limiter = createRateLimiter({ limit: 1, windowMs: 1000, now: () => clock });
    expect(limiter.check('a').allowed).toBe(true);
    expect(limiter.check('a').allowed).toBe(false);
    clock = 1000;
    expect(limiter.check('a').allowed).toBe(true);
  });

  it('reports remaining and a reset time within the window', () => {
    const limiter = createRateLimiter({ limit: 5, windowMs: 1000, now: () => 200 });
    const first = limiter.check('a');
    expect(first.remaining).toBe(4);
    expect(first.resetAt).toBe(1200);
  });

  it('sweeps stale keys so the map does not grow without bound', () => {
    let clock = 0;
    const limiter = createRateLimiter({ limit: 1, windowMs: 1000, now: () => clock });
    for (let i = 0; i < 100; i += 1) limiter.check(`key-${i}`);
    // Advance well past the window so a later check triggers a sweep, then the
    // old keys start fresh rather than staying blocked.
    clock = 5000;
    expect(limiter.check('key-0').allowed).toBe(true);
  });
});
