import { type Metadata } from 'next';
import { notFound } from 'next/navigation';

import { ContractSummary } from '@/components/contract-summary';
import { EventList } from '@/components/event-list';
import { isContractId } from '@/lib/contract-id';
import { truncateMiddle } from '@/lib/format';
import { GraphQLRequestError } from '@/lib/graphql/errors';
import { getContractWithEvents } from '@/lib/graphql/queries';
import { type ContractNode } from '@/lib/graphql/schemas';

/**
 * A contract's detail page.
 *
 * Fetches the contract together with its first page of events in a single
 * GraphQL round-trip, the nested traversal the REST surface cannot express
 * (ADR-043). The data is live, so the route is dynamic. A malformed id or an
 * untracked contract renders the segment's not-found UI; the interactive,
 * paginated, filterable event list is layered on top of this list in a later
 * step.
 */

export const dynamic = 'force-dynamic';

/** The first page the nested query pulls; the most recent events, newest first. */
const EVENT_PREVIEW_LIMIT = 25;

interface ContractPageProps {
  params: Promise<{ contractId: string }>;
}

/** Fetch the contract, mapping a not-found signal to `null` and rethrowing the rest. */
async function loadContract(id: string): Promise<ContractNode | null> {
  try {
    return await getContractWithEvents(id, { limit: EVENT_PREVIEW_LIMIT });
  } catch (error) {
    if (error instanceof GraphQLRequestError && error.isNotFound) return null;
    throw error;
  }
}

export async function generateMetadata({
  params,
}: ContractPageProps): Promise<Metadata> {
  const { contractId } = await params;
  return {
    title: isContractId(contractId)
      ? `Contract ${truncateMiddle(contractId)}`
      : 'Contract',
  };
}

export default async function ContractPage({ params }: ContractPageProps) {
  const { contractId } = await params;
  if (!isContractId(contractId)) notFound();

  const contract = await loadContract(contractId);
  if (contract === null) notFound();

  const events = contract.events?.items ?? [];
  const hasMore = contract.events?.nextCursor != null;

  return (
    <div className="flex flex-col gap-8">
      <ContractSummary contract={contract} />

      <section aria-labelledby="events-heading" className="flex flex-col gap-3">
        <div className="flex items-baseline justify-between gap-3">
          <h2 id="events-heading" className="text-sm font-medium">
            Events
          </h2>
          {events.length > 0 && (
            <span className="text-xs text-muted-foreground">
              {hasMore
                ? `Most recent ${events.length}`
                : `${events.length} total`}
            </span>
          )}
        </div>
        <EventList events={events} />
      </section>
    </div>
  );
}
