import { type InputHTMLAttributes } from 'react';

import { cn } from '@/lib/utils';

/**
 * The explorer's text input primitive, shadcn/ui new-york styling merged through
 * `cn` so a caller can override or extend the classes.
 */
export type InputProps = InputHTMLAttributes<HTMLInputElement>;

const BASE_CLASSES =
  'flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 ' +
  'text-sm ring-offset-background placeholder:text-muted-foreground ' +
  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ' +
  'focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50';

export function Input({ className, type = 'text', ...props }: InputProps) {
  return (
    <input type={type} className={cn(BASE_CLASSES, className)} {...props} />
  );
}
