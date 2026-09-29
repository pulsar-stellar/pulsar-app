import Link from 'next/link';

/**
 * Shown when a contract detail route resolves to nothing: an untracked contract
 * or a malformed id. Styled to match the app rather than the framework default.
 */
export default function ContractNotFound() {
  return (
    <div className="flex flex-col items-start gap-4">
      <h1 className="text-2xl font-semibold tracking-tight">
        Contract not found
      </h1>
      <p className="max-w-2xl text-muted-foreground">
        This contract is not being indexed, or the id is not a valid Soroban
        contract address. Check the id and try another.
      </p>
      <Link
        href="/"
        className="text-sm font-medium underline underline-offset-4"
      >
        Back to search
      </Link>
    </div>
  );
}
