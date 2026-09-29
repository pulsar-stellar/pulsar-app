export default function HomePage() {
  return (
    <div className="flex flex-col gap-6">
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

      <section
        aria-label="Coming soon"
        className="rounded-lg border border-dashed border-border p-6 text-sm text-muted-foreground"
      >
        The contract lookup lands next. This scaffold wires up the App Router,
        the design tokens, and the test harness the explorer screens build on.
      </section>
    </div>
  );
}
