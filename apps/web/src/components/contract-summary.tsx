import { StatusBadge } from '@/components/status-badge';
import { formatInteger, formatTimestamp } from '@/lib/format';
import { type ContractNode } from '@/lib/graphql/schemas';

/**
 * The header of a contract's detail page: its address, indexing status, and the
 * ledger window the indexer has covered.
 *
 * Presentational and server-rendered. It receives an already-validated
 * {@link ContractNode} and reads only its scalar fields; the nested event
 * connection, when present, is rendered by the event list beside it.
 */

interface MetaItemProps {
  label: string;
  children: React.ReactNode;
}

function MetaItem({ label, children }: MetaItemProps) {
  return (
    <div className="flex flex-col gap-1">
      <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        {label}
      </dt>
      <dd className="text-sm">{children}</dd>
    </div>
  );
}

interface ContractSummaryProps {
  contract: ContractNode;
}

export function ContractSummary({ contract }: ContractSummaryProps) {
  return (
    <section className="flex flex-col gap-4 rounded-lg border border-border p-5">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <h1 className="flex flex-col gap-1">
          <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            Contract
          </span>
          <span className="break-all font-mono text-sm">{contract.id}</span>
        </h1>
        <StatusBadge status={contract.status} />
      </div>

      <dl className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <MetaItem label="Added">{formatTimestamp(contract.addedAt)}</MetaItem>
        <MetaItem label="First indexed ledger">
          {contract.firstIndexedLedger === null
            ? 'Not yet indexed'
            : formatInteger(contract.firstIndexedLedger)}
        </MetaItem>
        <MetaItem label="Last indexed ledger">
          {formatInteger(contract.lastIndexedLedger)}
        </MetaItem>
      </dl>
    </section>
  );
}
