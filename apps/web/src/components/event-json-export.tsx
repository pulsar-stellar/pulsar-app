'use client';

import { useEffect, useRef, useState } from 'react';

import { Button } from '@/components/ui/button';

/**
 * Export the event as JSON: copy to the clipboard, or download a `.json` file.
 *
 * The event is serialized on the server and passed in as a string, so this
 * client island never imports the SDK-backed schemas and the ADR-046 bundle
 * boundary holds. The copy label flips to "Copied" on success and resets after a
 * short delay; the timer is cleared on unmount so it never updates a gone
 * component.
 */

interface EventJsonExportProps {
  /** The event already serialized as pretty-printed JSON. */
  json: string;
  /** The filename offered for the download. */
  filename: string;
}

/** How long the "Copied" confirmation stays up, in milliseconds. */
const COPIED_RESET_MS = 2000;

export function EventJsonExport({ json, filename }: EventJsonExportProps) {
  const [copied, setCopied] = useState(false);
  const resetTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (resetTimer.current !== null) clearTimeout(resetTimer.current);
    },
    [],
  );

  async function handleCopy(): Promise<void> {
    try {
      await navigator.clipboard.writeText(json);
      setCopied(true);
      resetTimer.current = setTimeout(() => setCopied(false), COPIED_RESET_MS);
    } catch {
      // Clipboard access can be denied; leave the label unchanged rather than
      // claiming a copy that did not happen.
      setCopied(false);
    }
  }

  function handleDownload(): void {
    const blob = new Blob([json], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = filename;
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    URL.revokeObjectURL(url);
  }

  return (
    <div className="flex items-center gap-2">
      <Button type="button" variant="outline" onClick={() => void handleCopy()}>
        {copied ? 'Copied' : 'Copy JSON'}
      </Button>
      <Button type="button" variant="outline" onClick={handleDownload}>
        Download
      </Button>
    </div>
  );
}
