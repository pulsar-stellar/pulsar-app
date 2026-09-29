import { ContractLookupForm } from '@/components/contract-lookup-form';

export default function HomePage() {
  return (
    <div className="flex flex-col gap-8">
      <section className="flex flex-col gap-3">
        <h1 className="text-3xl font-semibold tracking-tight">
          Explore Soroban contract events
        </h1>
        <p className="max-w-2xl text-muted-foreground">
          Pulsar Explorer reads decoded events for any Soroban contract on
          Stellar. Paste a contract ID to browse every event it has emitted,
          decoded and searchable, served by your Pulsar indexer.
        </p>
      </section>

      <section aria-labelledby="lookup-heading" className="flex flex-col gap-3">
        <h2 id="lookup-heading" className="text-sm font-medium">
          Look up a contract
        </h2>
        <ContractLookupForm />
        <p className="text-sm text-muted-foreground">
          A contract ID is 56 characters and starts with C.
        </p>
      </section>
    </div>
  );
}
