import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';

import { PulsarClient } from '../src/client.js';
import { PulsarNetworkError, PulsarValidationError } from '../src/errors.js';

import { eventPayload, trackedContractPayload } from './mocks/handlers.js';
import { INDEXER_URL, server } from './mocks/server.js';

const client = new PulsarClient({ indexerUrl: INDEXER_URL, timeoutMs: 200 });
const eventRoute = `${INDEXER_URL}/events/:eventId`;
const EVENT_ID = eventPayload.id;

/** Absence as ADR-019 defines it: a 404 and a not_found envelope together. */
const absent = () =>
  HttpResponse.json({ error: { code: 'not_found', message: 'no such event' } }, { status: 404 });

describe('fetching an event that exists', () => {
  it('returns the decoded event in camelCase', async () => {
    const event = await client.event(EVENT_ID);
    expect(event).toEqual({
      id: EVENT_ID,
      contractId: trackedContractPayload.id,
      ledger: eventPayload.ledger,
      txHash: eventPayload.tx_hash,
      eventIndex: 0,
      name: 'deposit',
      topics: eventPayload.topics_json,
      data: eventPayload.data_json,
      rawTopics: eventPayload.raw_topics,
      rawData: eventPayload.raw_data,
      emittedAt: eventPayload.emitted_at,
    });
  });

  it('requests the top-level event route with a GET, with no contract in the path', async () => {
    let seen: { method: string; url: string } | undefined;
    server.use(
      http.get(eventRoute, ({ request }) => {
        seen = { method: request.method, url: request.url };
        return HttpResponse.json({ data: eventPayload });
      }),
    );

    await client.event(EVENT_ID);
    expect(seen?.method).toBe('GET');
    expect(seen?.url).toBe(`${INDEXER_URL}/events/${EVENT_ID}`);
  });

  it('keeps an id past 2^53 exact, since it never becomes a number', async () => {
    const event = await client.event('9007199254740993');
    expect(event?.id).toBe('9007199254740993');
    expect(String(Number(event?.id))).not.toBe(event?.id);
  });
});

describe('fetching an event that does not exist', () => {
  it('returns null for a 404 carrying a not_found envelope', async () => {
    server.use(http.get(eventRoute, absent));
    expect(await client.event(EVENT_ID)).toBeNull();
  });

  it('returns exactly null and never undefined', async () => {
    server.use(http.get(eventRoute, absent));
    const event = await client.event(EVENT_ID);
    expect(event).not.toBeUndefined();
    expect(event === null).toBe(true);
  });

  it('throws on a bare 404, which is a routing problem rather than an absence', async () => {
    server.use(http.get(eventRoute, () => HttpResponse.json({}, { status: 404 })));
    try {
      await client.event(EVENT_ID);
      expect.unreachable('event should have thrown');
    } catch (error) {
      expect(error).toBeInstanceOf(PulsarNetworkError);
      expect((error as PulsarNetworkError).status).toBe(404);
    }
  });

  it('throws a validation error when a 200 carries a not_found envelope', async () => {
    server.use(
      http.get(eventRoute, () =>
        HttpResponse.json({ error: { code: 'not_found', message: 'contradictory' } }),
      ),
    );
    await expect(client.event(EVENT_ID)).rejects.toThrow(PulsarValidationError);
  });
});

describe('event id validation', () => {
  const failOnRequest = () =>
    server.use(
      http.get(eventRoute, () => {
        throw new Error('request should not have been sent');
      }),
    );

  it.each([
    ['an empty string', ''],
    ['whitespace only', '   '],
    ['an id with surrounding whitespace', ` ${EVENT_ID} `],
    ['a transaction hash pasted by mistake', 'ab'.repeat(32)],
    ['a negative number', '-1'],
    ['a decimal', '1.5'],
    ['a value that is not a string', undefined as never],
  ])('rejects %s before sending anything', async (_label, candidate) => {
    failOnRequest();
    await expect(client.event(candidate)).rejects.toThrow(PulsarValidationError);
  });

  it('accepts an id far past 2^53, which is the case the string form exists for', async () => {
    server.use(http.get(eventRoute, () => HttpResponse.json({ data: eventPayload })));
    await expect(client.event('99999999999999999999')).resolves.not.toBeNull();
  });

  it('names the operation and the offending id on the error', async () => {
    failOnRequest();
    try {
      await client.event('not-an-id');
      expect.unreachable('event should have thrown');
    } catch (error) {
      const validationError = error as PulsarValidationError;
      expect(validationError.operation).toBe('client.event');
      expect(validationError.details['eventId']).toBe('not-an-id');
    }
  });
});

describe('server failures', () => {
  it('throws on a server error', async () => {
    server.use(
      http.get(eventRoute, () =>
        HttpResponse.json({ error: { code: 'internal', message: 'db down' } }, { status: 500 }),
      ),
    );
    await expect(client.event(EVENT_ID)).rejects.toThrow(PulsarNetworkError);
  });

  it('throws when the event has the wrong shape', async () => {
    server.use(
      http.get(eventRoute, () => HttpResponse.json({ data: { ...eventPayload, ledger: 'soon' } })),
    );
    try {
      await client.event(EVENT_ID);
      expect.unreachable('event should have thrown');
    } catch (error) {
      expect((error as PulsarValidationError).issues[0]?.path.join('.')).toBe('ledger');
    }
  });

  it('rejects an event whose id came back as a number', async () => {
    server.use(
      http.get(eventRoute, () => HttpResponse.json({ data: { ...eventPayload, id: 42 } })),
    );
    await expect(client.event(EVENT_ID)).rejects.toThrow(PulsarValidationError);
  });
});
