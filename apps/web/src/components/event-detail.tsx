import Link from 'next/link';
import { type ReactNode } from 'react';

import { DecodedValueView } from '@/components/decoded-value';
import { EmptyState } from '@/components/empty-state';
import { formatInteger, formatTimestamp } from '@/lib/format';
import { type EventNode } from '@/lib/graphql/schemas';

/**
 * The event-detail body: identity, decoded topics and data, and the raw XDR
 * provenance.
 *
 * Presentational and server-rendered. The decoded topics and data go through
 * {@link DecodedValueView}, which owns the taxonomy rendering and the escaping
 * boundary (ADR-043); this component only lays out the sections. The contract id
 * links back to its detail page, and the raw base64 XDR is kept in a collapsed
 * disclosure as provenance, so the decoding can be checked rather than taken on
 * trust (ADR-023).
 */

/** An event with no decoded name degrades to this label (ADR-026). */
const UNNAMED_LABEL = 'Unnamed event';

interface EventDetailProps {
  event: EventNode;
}

/** One labelled field in the identity list. */
function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1">
      <dt className="text-xs font-medium text-muted-foreground">{label}</dt>
      <dd className="text-sm">{children}</dd>
    </div>
  );
}

export function EventDetail({ event }: EventDetailProps) {
  const named = event.name !== '';

  return (
    <article className="flex flex-col gap-8">
      <header className="flex flex-wrap items-center gap-3">
        <h1
          className={
            named
              ? 'text-2xl font-semibold tracking-tight'
              : 'text-2xl font-semibold italic tracking-tight text-muted-foreground'
          }
        >
          {named ? event.name : UNNAMED_LABEL}
        </h1>
        {!event.inSuccessfulContractCall && (
          <span className="rounded-full border border-amber-500 px-2 py-0.5 text-xs font-medium text-amber-700 dark:text-amber-400">
            Reverted
          </span>
        )}
      </header>

      <dl className="grid gap-4 sm:grid-cols-2">
        <Field label="Event ID">
          <code className="font-mono">{event.id}</code>
        </Field>
        <Field label="Contract">
          <Link
            href={`/c/${event.contractId}`}
            className="break-all font-mono underline underline-offset-4 hover:text-foreground"
          >
            {event.contractId}
          </Link>
        </Field>
        <Field label="Ledger">
          <code className="font-mono">{formatInteger(event.ledger)}</code>
        </Field>
        <Field label="Event index">
          <code className="font-mono">{formatInteger(event.eventIndex)}</code>
        </Field>
        <Field label="Transaction">
          <code className="break-all font-mono">{event.txHash}</code>
        </Field>
        <Field label="Emitted">{formatTimestamp(event.emittedAt)}</Field>
      </dl>

      <section aria-labelledby="topics-heading" className="flex flex-col gap-3">
        <h2 id="topics-heading" className="text-sm font-medium">
          Topics
        </h2>
        {event.topics.length === 0 ? (
          <EmptyState>No topics.</EmptyState>
        ) : (
          <ol className="flex flex-col gap-2">
            {event.topics.map((topic, index) => (
              <li
                key={index}
                className="rounded-md border border-border px-4 py-3"
              >
                <DecodedValueView value={topic} />
              </li>
            ))}
          </ol>
        )}
      </section>

      <section aria-labelledby="data-heading" className="flex flex-col gap-3">
        <h2 id="data-heading" className="text-sm font-medium">
          Data
        </h2>
        <div className="rounded-md border border-border px-4 py-3">
          <DecodedValueView value={event.data} />
        </div>
      </section>

      <section aria-labelledby="raw-heading" className="flex flex-col gap-3">
        <h2 id="raw-heading" className="text-sm font-medium">
          Raw XDR
        </h2>
        <details className="rounded-md border border-border px-4 py-3">
          <summary className="cursor-pointer select-none text-sm text-muted-foreground">
            Provenance (base64 XDR)
          </summary>
          <dl className="mt-3 flex flex-col gap-4">
            <Field label="Raw data">
              <code className="break-all font-mono text-xs">{event.rawData}</code>
            </Field>
            <div className="flex flex-col gap-1">
              <dt className="text-xs font-medium text-muted-foreground">
                Raw topics
              </dt>
              <dd>
                {event.rawTopics.length === 0 ? (
                  <span className="text-sm text-muted-foreground">None.</span>
                ) : (
                  <ol className="flex flex-col gap-1">
                    {event.rawTopics.map((raw, index) => (
                      <li key={index}>
                        <code className="break-all font-mono text-xs">{raw}</code>
                      </li>
                    ))}
                  </ol>
                )}
              </dd>
            </div>
          </dl>
        </details>
      </section>
    </article>
  );
}
