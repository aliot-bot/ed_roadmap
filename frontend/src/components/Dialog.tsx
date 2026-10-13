import { useEffect, useRef, type ReactNode } from 'react'
import { Icon } from './Icon'

export function Dialog({ title, children, onClose, labelledBy = 'dialog-title', className = '' }: { title: string; children: ReactNode; onClose: () => void; labelledBy?: string; className?: string }) {
  const dialogRef = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    const dialog = dialogRef.current
    dialog?.showModal()
    return () => dialog?.close()
  }, [])
  return (
    <dialog ref={dialogRef} className={`dialog ${className}`.trim()} aria-labelledby={labelledBy} onCancel={(event) => { event.preventDefault(); onClose() }} onClick={(event) => { if (event.target === dialogRef.current) onClose() }}>
      <div className="dialog-card">
        <div className="dialog-header">
          <h2 id={labelledBy}>{title}</h2>
          <button className="icon-button" type="button" onClick={onClose} aria-label="Закрыть"><Icon name="close" /></button>
        </div>
        {children}
      </div>
    </dialog>
  )
}

export function ErrorMessage({ error }: { error: unknown }) {
  if (!error) return null
  return <p className="form-error" role="alert">{error instanceof Error ? error.message : 'Произошла ошибка'}</p>
}
