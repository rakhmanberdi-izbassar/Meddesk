import type { ComponentProps } from 'react'
import { cn } from '../../lib/utils'

export function Badge({ className, ...props }: ComponentProps<'span'>) {
  return <span className={cn('inline-flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs font-semibold', className)} {...props} />
}
