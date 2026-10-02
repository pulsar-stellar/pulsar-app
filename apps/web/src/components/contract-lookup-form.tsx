'use client';

import { useRouter } from 'next/navigation';
import { useId, useState, type FormEvent } from 'react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { isContractId, normalizeContractId } from '@/lib/contract-id';

/**
 * The contract lookup: the explorer's front door.
 *
 * A pasted contract ID is validated against the same rule the SDK and indexer
 * enforce (see `@/lib/contract-id`), purely for immediate feedback, then the
 * form routes to that contract's page. Nothing is fetched here: the destination
 * page owns the lookup and its not-found handling, so this component stays a
 * small client island with no data dependency.
 *
 * The field carries a persistent format hint and, on a bad submit, an error,
 * both wired through `aria-describedby`; focus returns to the field so a
 * keyboard or screen-reader user can correct it without hunting for it.
 */
const INPUT_ID = 'contract-id';

export function ContractLookupForm() {
  const router = useRouter();
  const hintId = useId();
  const errorId = useId();
  const [value, setValue] = useState('');
  const [error, setError] = useState<string | null>(null);

  function handleSubmit(event: FormEvent<HTMLFormElement>): void {
    event.preventDefault();
    const id = normalizeContractId(value);
    if (!isContractId(id)) {
      setError('Enter a valid contract ID: 56 characters starting with C.');
      document.getElementById(INPUT_ID)?.focus();
      return;
    }
    setError(null);
    router.push(`/c/${id}`);
  }

  return (
    <form onSubmit={handleSubmit} noValidate className="flex flex-col gap-3">
      <div className="flex flex-col gap-2 sm:flex-row">
        <div className="flex-1">
          <label htmlFor={INPUT_ID} className="sr-only">
            Contract ID
          </label>
          <Input
            id={INPUT_ID}
            name={INPUT_ID}
            value={value}
            onChange={(event) => setValue(event.target.value)}
            placeholder="C..."
            autoComplete="off"
            autoCapitalize="characters"
            spellCheck={false}
            aria-invalid={error !== null}
            aria-describedby={error !== null ? `${hintId} ${errorId}` : hintId}
            className="font-mono"
          />
        </div>
        <Button type="submit" size="lg">
          Look up
        </Button>
      </div>
      <p id={hintId} className="text-sm text-muted-foreground">
        A contract ID is 56 characters and starts with C.
      </p>
      {error !== null && (
        <p id={errorId} role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
    </form>
  );
}
