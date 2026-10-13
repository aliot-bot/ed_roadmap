import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api'
import type { Roadmap } from '../types'
import { Dialog, ErrorMessage } from '../components/Dialog'
import { Icon } from '../components/Icon'
import { ProgressBar } from '../components/ProgressBar'
import { Button } from '../components/ui/Button'
import { Card } from '../components/ui/Card'
import { AnalyticsSummary } from '../components/AnalyticsSummary'

export function RoadmapsPage({ onOpen }: { onOpen: (id: string) => void }) {
  const queryClient = useQueryClient()
  const roadmaps = useQuery({ queryKey: ['roadmaps'], queryFn: api.listRoadmaps })
  const analytics = useQuery({ queryKey: ['analytics'], queryFn: api.getAnalytics })
  const [createOpen, setCreateOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [editing, setEditing] = useState<Roadmap | null>(null)
  const [deleting, setDeleting] = useState<Roadmap | null>(null)
  const [coverRevision, setCoverRevision] = useState(0)
  const invalidate = () => Promise.all([
    queryClient.invalidateQueries({ queryKey: ['roadmaps'] }),
    queryClient.invalidateQueries({ queryKey: ['analytics'] }),
  ])
  const remove = useMutation({ mutationFn: (id: string) => api.deleteRoadmap(id), onSuccess: () => { void invalidate(); setDeleting(null) } })
  const normalizedSearch = search.trim().toLocaleLowerCase('ru')
  const visibleRoadmaps = roadmaps.data?.filter((item) => `${item.title} ${item.description}`.toLocaleLowerCase('ru').includes(normalizedSearch)) ?? []

  return (
    <div className="page roadmaps-page">
      <div className="page-title-row">
        <div>
          <p className="eyebrow">Карты знаний</p>
          <h1>Мои карты</h1>
        </div>
        <Button variant="primary" onClick={() => setCreateOpen(true)}><Icon name="plus" /> Новая карта</Button>
      </div>

      <AnalyticsSummary analytics={analytics.data} isLoading={analytics.isPending} isError={analytics.isError} />

      {roadmaps.isPending && <CardGridSkeleton />}
      {roadmaps.isError && <section className="empty-state"><h2>Карты не загрузились</h2><ErrorMessage error={roadmaps.error} /><button className="button secondary" onClick={() => void roadmaps.refetch()}>Повторить</button></section>}
      {roadmaps.data?.length === 0 && (
        <section className="empty-state">
          <span className="empty-illustration"><Icon name="map" /></span>
          <h2>Нет карт</h2>
          <Button variant="primary" onClick={() => setCreateOpen(true)}><Icon name="plus" /> Создать карту</Button>
        </section>
      )}
      {!!roadmaps.data?.length && (
        <section className="roadmap-library" aria-labelledby="all-roadmaps-title">
          <div className="section-heading">
            <div><p className="eyebrow">Библиотека</p><h2 id="all-roadmaps-title">Все карты</h2></div>
            <label className="roadmap-search"><Icon name="search" /><input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Найти карту" aria-label="Найти карту" /></label>
          </div>
          {visibleRoadmaps.length ? <div className="roadmap-grid">
          {visibleRoadmaps.map((item, index) => (
            <Card key={item.id} className="roadmap-card">
              <button className="card-main" onClick={() => onOpen(item.id)} aria-label={`Открыть карту «${item.title}»`}>
                <MapPreview variant={index % 3} coverUrl={item.has_cover ? `/api/roadmaps/${item.id}/cover?v=${coverRevision}` : undefined} />
                <span className="card-copy"><h2>{item.title}</h2><p>{item.description || 'Добавьте описание, чтобы зафиксировать цель карты.'}</p></span>
                <span className="card-progress"><ProgressBar progress={item.progress} compact /></span>
              </button>
              <div className="card-actions">
                <span className="card-action-hint">Открыть карту</span>
                <button className="icon-button" onClick={() => setEditing(item)} aria-label={`Изменить карту «${item.title}»`}><Icon name="edit" /></button>
                <button className="icon-button danger" onClick={() => setDeleting(item)} aria-label={`Удалить «${item.title}»`}><Icon name="trash" /></button>
              </div>
            </Card>
          ))}
          </div> : <div className="search-empty"><Icon name="search" /><h3>Ничего не найдено</h3><p>Попробуйте изменить запрос.</p></div>}
        </section>
      )}
      {createOpen && <RoadmapForm title="Новая карта" onClose={() => setCreateOpen(false)} onSaved={() => { setCreateOpen(false); setCoverRevision((value) => value + 1); void invalidate() }} />}
      {editing && <RoadmapForm title="Изменить карту" roadmap={editing} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); setCoverRevision((value) => value + 1); void invalidate() }} />}
      {deleting && (
        <Dialog title="Удалить карту?" onClose={() => setDeleting(null)}>
          <div className="dialog-body">
            <p>«{deleting.title}» будет удалена вместе со всеми темами, связями, заметками и прогрессом. Это действие нельзя отменить.</p>
            <ErrorMessage error={remove.error} />
            <div className="dialog-actions">
              <button className="button secondary" onClick={() => setDeleting(null)}>Отмена</button>
              <button className="button danger" disabled={remove.isPending} onClick={() => remove.mutate(deleting.id)}>Удалить карту</button>
            </div>
          </div>
        </Dialog>
      )}
    </div>
  )
}

function RoadmapForm({ title, roadmap, onClose, onSaved }: { title: string; roadmap?: Roadmap; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(roadmap?.title ?? '')
  const [description, setDescription] = useState(roadmap?.description ?? '')
  const [coverFile, setCoverFile] = useState<File>()
  const [removeCover, setRemoveCover] = useState(false)
  const coverPreview = useMemo(() => coverFile ? URL.createObjectURL(coverFile) : undefined, [coverFile])
  useEffect(() => {
    return () => { if (coverPreview) URL.revokeObjectURL(coverPreview) }
  }, [coverPreview])
  const mutation = useMutation({
    mutationFn: async () => {
      const saved = roadmap ? await api.updateRoadmap(roadmap.id, { title: name.trim(), description: description.trim() }) : await api.createRoadmap({ title: name.trim(), description: description.trim() })
      if (coverFile) await api.uploadRoadmapCover(saved.id, coverFile)
      else if (roadmap && removeCover) await api.deleteRoadmapCover(roadmap.id)
      return saved
    },
    onSuccess: onSaved,
  })
  const submit = (event: FormEvent) => { event.preventDefault(); if (name.trim()) mutation.mutate() }
  return (
    <Dialog title={title} onClose={onClose}>
      <form className="dialog-body form-stack" onSubmit={submit}>
        <label>Название<input value={name} onChange={(event) => setName(event.target.value)} maxLength={120} required autoFocus /></label>
          <label>Короткое описание<textarea value={description} onChange={(event) => setDescription(event.target.value)} maxLength={500} rows={3} placeholder="Чему посвящена эта карта?" /></label>
          <div className="cover-picker">
            <label className="field-label" htmlFor="roadmap-cover">Изображение карты</label>
            {(coverPreview || (roadmap?.has_cover && !removeCover)) && <img className="cover-picker-preview" src={coverPreview || `/api/roadmaps/${roadmap?.id}/cover`} alt="" />}
            <input id="roadmap-cover" type="file" accept="image/jpeg,image/png,image/gif,image/webp" onChange={(event) => { setCoverFile(event.target.files?.[0]); setRemoveCover(false) }} />
            <span className="field-hint">JPEG, PNG, GIF или WebP, до 5 МБ.</span>
            {roadmap?.has_cover && <button type="button" className="text-button cover-remove" onClick={() => { setRemoveCover(true); setCoverFile(undefined) }}>{removeCover ? 'Изображение будет удалено' : 'Убрать изображение'}</button>}
          </div>
        <ErrorMessage error={mutation.error} />
        <div className="dialog-actions"><button type="button" className="button secondary" onClick={onClose}>Отмена</button><button className="button primary" disabled={mutation.isPending}>{mutation.isPending ? 'Сохраняем…' : 'Сохранить'}</button></div>
      </form>
    </Dialog>
  )
}

function CardGridSkeleton() {
  return <div className="roadmap-grid" role="status" aria-live="polite" aria-label="Загрузка карт">{[0, 1, 2].map((item) => <div className="roadmap-card skeleton" key={item} />)}</div>
}

function MapPreview({ variant, coverUrl }: { variant: number; coverUrl?: string }) {
  return (
    <span className={`map-preview preview-${variant}`} aria-hidden="true">
      {coverUrl ? <img src={coverUrl} alt="" /> : <svg viewBox="0 0 320 180" fill="none">
        {variant === 0 && <><path d="M58 91h64l33-39h78l34 45" /><path d="M122 91l32 42h74" /><circle cx="58" cy="91" r="13" /><circle cx="155" cy="52" r="13" /><circle cx="267" cy="97" r="13" /><circle cx="154" cy="133" r="13" /><circle cx="228" cy="133" r="13" /></>}
        {variant === 1 && <><path d="M51 126V73h67l34 36h116" /><path d="M118 73l46-38h65" /><circle cx="51" cy="126" r="13" /><circle cx="51" cy="73" r="13" /><circle cx="118" cy="73" r="13" /><circle cx="164" cy="35" r="13" /><circle cx="229" cy="35" r="13" /><circle cx="268" cy="109" r="13" /></>}
        {variant === 2 && <><path d="M54 48h88l34 43h89" /><path d="M142 48l47 82h76" /><path d="M98 137V91h78" /><circle cx="54" cy="48" r="13" /><circle cx="142" cy="48" r="13" /><circle cx="176" cy="91" r="13" /><circle cx="265" cy="91" r="13" /><circle cx="98" cy="137" r="13" /><circle cx="189" cy="130" r="13" /><circle cx="265" cy="130" r="13" /></>}
      </svg>}
    </span>
  )
}
