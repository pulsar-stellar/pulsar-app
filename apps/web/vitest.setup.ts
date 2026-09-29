import '@testing-library/jest-dom/vitest';

import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';

// Unmount and clear the DOM between tests so no render leaks into the next one.
afterEach(() => {
  cleanup();
});
