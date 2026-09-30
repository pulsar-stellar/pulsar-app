import { type Metadata } from 'next';
import { notFound } from 'next/navigation';

import { ContractSummary } from '@/components/contract-summary';
import { EventsPanel } from '@/components/events-panel';
import { isContractId } from '@/lib/contract-id';
import { eventFiltersToQuery, readEventFilters, type EventFilters } from '@/lib/events-filters';
import { truncateMiddle } from '@/lib/format';
import { GraphQLRequestError } from '@/lib/graphql/errors';
import { getContractWithEvents } from '@/lib/graphql/queries';
import { type ContractNode } from '@/lib/graphql/schemas';

/**
 * A contract's detail page.
 *
 * Fetches the contract together with its first page of events in a single
 * GraphQL round-trip, the nested traversal the REST surface cannot express
 * (ADR-043). The event filters live in the URL query string, so a filtered view
 * is shareable and survives the back button; the page parses them, fetches the
 * matching first page server-side, and hands both to the interactive panel,
 * which owns filter navigation and cursor pagination from there. The data is
 * live, so the route is dynamic. A malformed id or an untracked contract
 * renders the segment's not-found UI.
 */

export const dynamic = 'force-dynamic';

/** The page size for the first (server-rendered) page and each "Load more". */
const EVENT_PAGE_SIZE = 25;

type SearchParams = Record<string, string | string[] | undefined>;

interface ContractPageProps {
  params: Promise<{ contractId: string }>;
  searchParams: Promise<SearchParams>;
}

/** Collapse Next's `searchParams` shape into a plain `URLSearchParams`. */
function toSearchParams(raw: SearchParams): URLSearchParams {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(raw)) {
    if (typeof value === 'string') params.set(key, value);
    else if (Array.isArray(value) && value[0] !== undefined) params.set(key, value[0]);
  }
  return params;
}

/** Fetch the contract and its first event page, mapping not-found to `null`. */
async function loadContract(
  id: string,
  filters: EventFilters,
): Promise<ContractNode | null> {
  try {
    return await getContractWithEvents(id, { ...filters, limit: EVENT_PAGE_SIZE });
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

export default async function ContractPage({
  params,
  searchParams,
}: ContractPageProps) {
  const { contractId } = await params;
  if (!isContractId(contractId)) notFound();

  const filters = readEventFilters(toSearchParams(await searchParams));

  const contract = await loadContract(contractId, filters);
  if (contract === null) notFound();

  const events = contract.events?.items ?? [];
  const cursor = contract.events?.nextCursor ?? null;

  return (
    <div className="flex flex-col gap-8">
      <ContractSummary contract={contract} />

      <section aria-labelledby="events-heading" className="flex flex-col gap-3">
        <h2 id="events-heading" className="text-sm font-medium">
          Events
        </h2>
        <EventsPanel
          key={eventFiltersToQuery(filters)}
          contractId={contractId}
          filters={filters}
          initialEvents={events}
          initialCursor={cursor}
        />
      </section>
    </div>
  );
}
