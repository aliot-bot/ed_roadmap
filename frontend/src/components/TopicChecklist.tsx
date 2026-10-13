import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import type { ChecklistItem, ChecklistItemInput } from '../types'
import { Icon } from './Icon'
import { ErrorMessage } from './Dialog'

export function TopicChecklist({ topicId }: { topicId: string }) {
  const queryClient = useQueryClient()
  const checklist = useQuery({ queryKey: ['checklist', topicId], queryFn: () => api.getChecklist(topicId) })
  const [draft, setDraft] = useState('')
  const save = useMutation({
    mutationFn: (items: ChecklistItemInput[]) => api.setChecklist(topicId, items),
    onSuccess: (items) => queryClient.setQueryData(['checklist', topicId], items),
  })
  const items = checklist.data ?? []
  const update = (next: ChecklistItemInput[]) => save.mutate(next)
  const inputs = (next: ChecklistItem[] = items): ChecklistItemInput[] => next.map((item, index) => ({ id: item.id, text: item.text, completed: item.completed, sort_order: index }))
  const add = (event: FormEvent) => {
    event.preventDefault()
    const text = draft.trim()
    if (!text || save.isPending) return
    update([...inputs(), { text, completed: false, sort_order: items.length }])
    setDraft('')
  }
  const completed = items.filter((item) => item.completed).length
  return <section className="topic-checklist" aria-label="Чек-лист темы">
    <div className="checklist-heading"><strong>Чек-лист</strong>{items.length > 0 && <span>{completed}/{items.length}</span>}</div>
    {checklist.isPending ? <p className="field-hint">Загрузка чек-листа…</p> : items.length > 0 && <ul>{items.map((item) => <li key={item.id}><label><input type="checkbox" checked={item.completed} disabled={save.isPending} onChange={(event) => update(inputs(items.map((current) => current.id === item.id ? { ...current, completed: event.target.checked } : current)))} /><span>{item.text}</span></label><button className="icon-button danger" type="button" aria-label={`Удалить пункт «${item.text}»`} disabled={save.isPending} onClick={() => update(inputs(items.filter((current) => current.id !== item.id)))}><Icon name="close" /></button></li>)}</ul>}
    <form className="checklist-add" onSubmit={add}><input value={draft} onChange={(event) => setDraft(event.target.value)} maxLength={500} placeholder="Добавить пункт" aria-label="Новый пункт чек-листа" /><button className="button tertiary" disabled={!draft.trim() || save.isPending}><Icon name="plus" /> Добавить</button></form>
    <ErrorMessage error={checklist.error ?? save.error} />
  </section>
}
