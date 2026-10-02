import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { DecodedValueView } from '@/components/decoded-value';
import { type DecodedValue } from '@/lib/graphql/schemas';

const ADDRESS = 'CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L';

describe('DecodedValueView scalars', () => {
  it.each<{ name: string; value: DecodedValue; text: string; tag: string }>([
    { name: 'u32', value: { type: 'u32', value: 4294967295 }, text: '4294967295', tag: 'u32' },
    { name: 'i32', value: { type: 'i32', value: -2147483648 }, text: '-2147483648', tag: 'i32' },
    { name: 'u64', value: { type: 'u64', value: '18446744073709551615' }, text: '18446744073709551615', tag: 'u64' },
    { name: 'i64', value: { type: 'i64', value: '-9223372036854775808' }, text: '-9223372036854775808', tag: 'i64' },
    { name: 'u128', value: { type: 'u128', value: '340282366920938463463374607431768211455' }, text: '340282366920938463463374607431768211455', tag: 'u128' },
    { name: 'i128', value: { type: 'i128', value: '170141183460469231731687303715884105727' }, text: '170141183460469231731687303715884105727', tag: 'i128' },
    { name: 'u256', value: { type: 'u256', value: '115792089237316195423570985008687907853' }, text: '115792089237316195423570985008687907853', tag: 'u256' },
    { name: 'i256', value: { type: 'i256', value: '-5' }, text: '-5', tag: 'i256' },
    { name: 'address', value: { type: 'address', value: ADDRESS }, text: ADDRESS, tag: 'address' },
    { name: 'symbol', value: { type: 'symbol', value: 'transfer' }, text: 'transfer', tag: 'symbol' },
    { name: 'bool true', value: { type: 'bool', value: true }, text: 'true', tag: 'bool' },
    { name: 'bool false', value: { type: 'bool', value: false }, text: 'false', tag: 'bool' },
  ])('renders a $name with its value and a type tag', ({ value, text, tag }) => {
    render(<DecodedValueView value={value} />);
    expect(screen.getByText(text)).toBeInTheDocument();
    expect(screen.getByText(tag)).toBeInTheDocument();
  });

  it('prefixes bytes with 0x', () => {
    render(<DecodedValueView value={{ type: 'bytes', value: 'deadbeef' }} />);
    expect(screen.getByText('0xdeadbeef')).toBeInTheDocument();
    expect(screen.getByText('bytes')).toBeInTheDocument();
  });

  it('does not double-prefix bytes already carrying 0x', () => {
    render(<DecodedValueView value={{ type: 'bytes', value: '0xcafe' }} />);
    expect(screen.getByText('0xcafe')).toBeInTheDocument();
  });

  it('renders void with no value', () => {
    render(<DecodedValueView value={{ type: 'void' }} />);
    expect(screen.getByText('void')).toBeInTheDocument();
  });

  it('renders the raw xdr of an unknown value', () => {
    render(<DecodedValueView value={{ type: 'unknown', xdr: 'AAAABQ==' }} />);
    expect(screen.getByText('unknown')).toBeInTheDocument();
    expect(screen.getByText('AAAABQ==')).toBeInTheDocument();
  });
});

describe('DecodedValueView derived hints', () => {
  it('shows a derived UTC date beside a timepoint, keeping the raw seconds', () => {
    render(<DecodedValueView value={{ type: 'timepoint', value: '1609459200' }} />);
    expect(screen.getByText('1609459200')).toBeInTheDocument();
    expect(screen.getByText(/Jan 01, 2021/)).toBeInTheDocument();
  });

  it('shows a derived span beside a duration, keeping the raw seconds', () => {
    render(<DecodedValueView value={{ type: 'duration', value: '3661' }} />);
    expect(screen.getByText('3661')).toBeInTheDocument();
    expect(screen.getByText(/1h 1m 1s/)).toBeInTheDocument();
  });

  it('omits the hint when a timepoint is not a safe second count', () => {
    render(<DecodedValueView value={{ type: 'timepoint', value: '99999999999999999999' }} />);
    expect(screen.getByText('99999999999999999999')).toBeInTheDocument();
    expect(screen.queryByText(/UTC/)).not.toBeInTheDocument();
  });
});

describe('DecodedValueView escaping (ADR-043: escaping is the explorer boundary)', () => {
  it('renders a decoded string with HTML as literal text, creating no element', () => {
    const payload = '<img src=x onerror="alert(1)">';
    const { container } = render(<DecodedValueView value={{ type: 'string', value: payload }} />);
    expect(container.querySelector('img')).toBeNull();
    expect(screen.getByText(payload)).toBeInTheDocument();
  });

  it('renders a decoded string with a script tag as literal text, creating no script node', () => {
    const payload = '<script>alert(document.cookie)</script>';
    const { container } = render(<DecodedValueView value={{ type: 'string', value: payload }} />);
    expect(container.querySelector('script')).toBeNull();
    expect(screen.getByText(payload)).toBeInTheDocument();
  });

  it('escapes a malicious value inside a nested map key, proving recursion escapes too', () => {
    const payload = '<img src=x onerror="alert(1)">';
    const { container } = render(
      <DecodedValueView
        value={{
          type: 'map',
          value: [
            { key: { type: 'string', value: payload }, value: { type: 'u32', value: 1 } },
          ],
        }}
      />,
    );
    expect(container.querySelector('img')).toBeNull();
    expect(screen.getByText(payload)).toBeInTheDocument();
  });

  it('escapes a malicious value nested inside a vec', () => {
    const payload = '<img src=x onerror="alert(1)">';
    const { container } = render(
      <DecodedValueView value={{ type: 'vec', value: [{ type: 'string', value: payload }] }} />,
    );
    expect(container.querySelector('img')).toBeNull();
    expect(screen.getByText(payload)).toBeInTheDocument();
  });

  it('escapes a script payload across every string-bearing scalar variant', () => {
    const payload = '<script>alert(1)</script>';
    const cases: DecodedValue[] = [
      { type: 'symbol', value: payload },
      { type: 'address', value: payload },
      { type: 'bytes', value: payload },
      { type: 'unknown', xdr: payload },
    ];
    for (const value of cases) {
      const { container, unmount } = render(<DecodedValueView value={value} />);
      expect(container.querySelector('script')).toBeNull();
      unmount();
    }
  });
});

describe('DecodedValueView nesting', () => {
  it('renders a vec as a disclosure listing each element', () => {
    const { container } = render(
      <DecodedValueView
        value={{
          type: 'vec',
          value: [
            { type: 'symbol', value: 'alpha' },
            { type: 'symbol', value: 'beta' },
          ],
        }}
      />,
    );
    const details = container.querySelector('details');
    expect(details).not.toBeNull();
    expect(details).toHaveAttribute('open');
    expect(screen.getByText('alpha')).toBeInTheDocument();
    expect(screen.getByText('beta')).toBeInTheDocument();
    // Summary names the kind and the count.
    expect(screen.getByText(/vec/)).toBeInTheDocument();
    expect(screen.getByText(/2 items/)).toBeInTheDocument();
  });

  it('renders a tuple as a disclosure', () => {
    render(
      <DecodedValueView
        value={{ type: 'tuple', value: [{ type: 'u32', value: 7 }] }}
      />,
    );
    expect(screen.getByText(/tuple/)).toBeInTheDocument();
    expect(screen.getByText('7')).toBeInTheDocument();
  });

  it('renders a map as key/value pairs', () => {
    render(
      <DecodedValueView
        value={{
          type: 'map',
          value: [
            { key: { type: 'symbol', value: 'to' }, value: { type: 'address', value: ADDRESS } },
          ],
        }}
      />,
    );
    expect(screen.getByText(/map/)).toBeInTheDocument();
    expect(screen.getByText('to')).toBeInTheDocument();
    expect(screen.getByText(ADDRESS)).toBeInTheDocument();
  });

  it('renders an empty vec without crashing', () => {
    render(<DecodedValueView value={{ type: 'vec', value: [] }} />);
    expect(screen.getByText(/0 items/)).toBeInTheDocument();
  });
});
