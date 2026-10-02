import { fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { EventJsonExport } from '@/components/event-json-export';

const JSON_TEXT = '{\n  "id": "42"\n}';

afterEach(() => {
  vi.restoreAllMocks();
});

describe('EventJsonExport', () => {
  it('copies the JSON to the clipboard and shows feedback', async () => {
    const writeText = vi.fn<(text: string) => Promise<void>>().mockResolvedValue();
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    });

    render(<EventJsonExport json={JSON_TEXT} filename="event-42.json" />);
    fireEvent.click(screen.getByRole('button', { name: /copy json/i }));

    expect(writeText).toHaveBeenCalledWith(JSON_TEXT);
    expect(await screen.findByRole('button', { name: /copied/i })).toBeInTheDocument();
    expect(screen.getByText('Copied to clipboard')).toBeInTheDocument();
  });

  it('keeps the original label when the copy fails', async () => {
    const writeText = vi
      .fn<(text: string) => Promise<void>>()
      .mockRejectedValue(new Error('denied'));
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    });

    render(<EventJsonExport json={JSON_TEXT} filename="event-42.json" />);
    fireEvent.click(screen.getByRole('button', { name: /copy json/i }));

    // The promise rejects; the label must not flip to "Copied".
    expect(await screen.findByText('Copy failed')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /copy json/i })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /copied/i })).not.toBeInTheDocument();
  });

  it('downloads the JSON as a named file', () => {
    const createObjectURL = vi.fn((_blob: Blob | MediaSource) => 'blob:mock');
    const revokeObjectURL = vi.fn((_url: string) => undefined);
    URL.createObjectURL = createObjectURL;
    URL.revokeObjectURL = revokeObjectURL;
    const clickSpy = vi
      .spyOn(HTMLAnchorElement.prototype, 'click')
      .mockImplementation(() => undefined);

    render(<EventJsonExport json={JSON_TEXT} filename="event-42.json" />);
    fireEvent.click(screen.getByRole('button', { name: /download/i }));

    expect(createObjectURL).toHaveBeenCalledOnce();
    expect(createObjectURL.mock.calls[0]?.[0]).toBeInstanceOf(Blob);
    expect(clickSpy).toHaveBeenCalledOnce();
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:mock');
  });
});
