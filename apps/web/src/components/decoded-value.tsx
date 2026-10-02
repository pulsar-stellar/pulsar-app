import { type ReactNode } from 'react';

import {
  formatDurationSeconds,
  formatUnixSeconds,
} from '@/lib/format';
import { type DecodedValue } from '@/lib/graphql/schemas';

/**
 * Renders one decoded Soroban value from the ADR-023 taxonomy, recursing through
 * `vec`, `map`, and `tuple`.
 *
 * Security boundary (ADR-043): decoded values carry arbitrary contract content,
 * which is untrusted. Every string this renders, a `string`, `symbol`, `bytes`,
 * `address`, a map key, or an `unknown` value's XDR, is placed as a React text
 * child, so React escapes it. Nothing decoded is ever passed to
 * `dangerouslySetInnerHTML` or into an attribute sink (`href`, `src`, `style`),
 * so a value like `<img onerror=...>` shows as literal text and can never become
 * a DOM node. "Correct escaping on render is the explorer's responsibility," and
 * this is where that responsibility is met; the escaping tests guard it.
 *
 * Pure and server-rendered: it holds no state and ships no client JavaScript.
 * Nesting uses native `<details>`, open by default, so a deep value is fully
 * visible yet collapsible with no script. The `DecodedValue` import is type-only
 * (ADR-046), so the SDK never reaches a bundle through this module.
 */

interface DecodedValueViewProps {
  value: DecodedValue;
}

/** A small muted label naming the value's decoded type. */
function TypeTag({ type }: { type: string }) {
  return (
    <span className="shrink-0 rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-muted-foreground">
      {type}
    </span>
  );
}

/** One scalar row: the type tag beside the value's presentation. */
function Row({ tag, children }: { tag: string; children: ReactNode }) {
  return (
    <div className="flex flex-wrap items-baseline gap-2">
      <TypeTag type={tag} />
      {children}
    </div>
  );
}

/** Monospace presentation for identifiers, numbers, hashes, and raw bytes. */
function Mono({ children }: { children: ReactNode }) {
  return <code className="break-all font-mono text-sm">{children}</code>;
}

/** A muted gloss shown beside a raw value (a derived date or span). */
function Hint({ children }: { children: ReactNode }) {
  return <span className="text-xs text-muted-foreground">({children})</span>;
}

/** Strip a leading `0x` so hex bytes render with exactly one prefix. */
function hexBody(value: string): string {
  return value.startsWith('0x') ? value.slice(2) : value;
}

/** A disclosure wrapping an ordered sequence (`vec` or `tuple`). */
function SequenceView({
  tag,
  items,
}: {
  tag: string;
  items: readonly DecodedValue[];
}) {
  return (
    <details open className="group">
      <summary className="flex cursor-pointer select-none items-baseline gap-2">
        <TypeTag type={tag} />
        <span className="text-xs text-muted-foreground">
          {items.length} item{items.length === 1 ? '' : 's'}
        </span>
      </summary>
      <ol className="mt-2 flex flex-col gap-2 border-l border-border pl-4">
        {items.map((item, index) => (
          <li key={index} className="flex items-baseline gap-2">
            <span className="shrink-0 font-mono text-xs text-muted-foreground">
              #{index}
            </span>
            <DecodedValueView value={item} />
          </li>
        ))}
      </ol>
    </details>
  );
}

export function DecodedValueView({ value }: DecodedValueViewProps) {
  switch (value.type) {
    case 'address':
    case 'symbol':
      return (
        <Row tag={value.type}>
          <Mono>{value.value}</Mono>
        </Row>
      );
    case 'string':
      // Free-form, untrusted text: rendered as an escaped text child, not mono.
      return (
        <Row tag="string">
          <span className="break-words text-sm">{value.value}</span>
        </Row>
      );
    case 'bool':
      return (
        <Row tag="bool">
          <Mono>{value.value ? 'true' : 'false'}</Mono>
        </Row>
      );
    case 'bytes':
      return (
        <Row tag="bytes">
          <Mono>0x{hexBody(value.value)}</Mono>
        </Row>
      );
    case 'u32':
    case 'i32':
      return (
        <Row tag={value.type}>
          <Mono>{String(value.value)}</Mono>
        </Row>
      );
    case 'u64':
    case 'i64':
    case 'u128':
    case 'i128':
    case 'u256':
    case 'i256':
      return (
        <Row tag={value.type}>
          <Mono>{value.value}</Mono>
        </Row>
      );
    case 'timepoint': {
      const derived = formatUnixSeconds(value.value);
      return (
        <Row tag="timepoint">
          <Mono>{value.value}</Mono>
          {derived !== null && <Hint>{derived}</Hint>}
        </Row>
      );
    }
    case 'duration': {
      const derived = formatDurationSeconds(value.value);
      return (
        <Row tag="duration">
          <Mono>{value.value}</Mono>
          {derived !== null && <Hint>{derived}</Hint>}
        </Row>
      );
    }
    case 'void':
      return (
        <Row tag="void">
          <span className="text-sm text-muted-foreground">no value</span>
        </Row>
      );
    case 'unknown':
      // Undecodable by this version: show the raw base64 XDR (escaped) so the
      // value is still inspectable rather than hidden.
      return (
        <Row tag="unknown">
          <Mono>{value.xdr}</Mono>
          <span className="text-xs text-muted-foreground">(undecodable)</span>
        </Row>
      );
    case 'vec':
      return <SequenceView tag="vec" items={value.value} />;
    case 'tuple':
      return <SequenceView tag="tuple" items={value.value} />;
    case 'map':
      return (
        <details open className="group">
          <summary className="flex cursor-pointer select-none items-baseline gap-2">
            <TypeTag type="map" />
            <span className="text-xs text-muted-foreground">
              {value.value.length} entr{value.value.length === 1 ? 'y' : 'ies'}
            </span>
          </summary>
          <ul className="mt-2 flex flex-col gap-3 border-l border-border pl-4">
            {value.value.map((entry, index) => (
              <li key={index} className="flex flex-col gap-1">
                <div className="flex items-baseline gap-2">
                  <span className="shrink-0 font-mono text-xs text-muted-foreground">
                    key
                  </span>
                  <DecodedValueView value={entry.key} />
                </div>
                <div className="flex items-baseline gap-2">
                  <span className="shrink-0 font-mono text-xs text-muted-foreground">
                    val
                  </span>
                  <DecodedValueView value={entry.value} />
                </div>
              </li>
            ))}
          </ul>
        </details>
      );
    default:
      // The union is closed, so this is unreachable; kept as a defensive,
      // still-escaped fallback rather than crashing a page on a surprise shape.
      return (
        <Row tag="unsupported">
          <Mono>{JSON.stringify(value)}</Mono>
        </Row>
      );
  }
}
