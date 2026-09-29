import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

/**
 * Merge class names, resolving Tailwind conflicts so the last utility wins.
 *
 * This is the standard shadcn/ui helper: `clsx` handles conditional and array
 * inputs, `tailwind-merge` collapses conflicting Tailwind classes (for example
 * `px-2 px-4` becomes `px-4`) so callers can pass overrides without ordering
 * surprises.
 */
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}
