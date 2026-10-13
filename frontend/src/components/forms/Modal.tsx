import * as Dialog from '@radix-ui/react-dialog'
import { X } from 'lucide-react'
import { useRef, type ReactNode } from 'react'
import { useLocale } from '../../lib/locale'

export function Modal({ title, description, children, onClose, busy = false }: { title: string; description: string; children: ReactNode; onClose: () => void; busy?: boolean }) {
  const { t } = useLocale()
  const content = useRef<HTMLDivElement>(null)
  const opener = useRef(document.activeElement instanceof HTMLElement ? document.activeElement : null)
  return <Dialog.Root open onOpenChange={open => { if (!open && !busy) onClose() }}><Dialog.Portal><Dialog.Overlay className="drawer-overlay" /><Dialog.Content ref={content} onOpenAutoFocus={event => { const target = content.current?.querySelector<HTMLElement>('input:not([disabled]), select:not([disabled]), .form-actions button'); if (target) { event.preventDefault(); target.focus() } }} onCloseAutoFocus={event => { event.preventDefault(); if (opener.current?.isConnected) opener.current.focus() }} className="form-modal" onEscapeKeyDown={event => { if (busy) event.preventDefault() }} onPointerDownOutside={event => { if (busy) event.preventDefault() }}><Dialog.Title className="modal-title">{title}</Dialog.Title><Dialog.Description className="modal-description">{description}</Dialog.Description><button className="modal-close" disabled={busy} onClick={onClose} aria-label={t.close}><X size={20} aria-hidden="true" /></button>{children}</Dialog.Content></Dialog.Portal></Dialog.Root>
}
