import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { api } from '../api'
import { Dialog, ErrorMessage } from './Dialog'
import { Icon } from './Icon'

type DocumentMode = 'edit' | 'preview'

export function TopicDocument({ topicId }: { topicId: string }) {
  const queryClient = useQueryClient()
  const documentQuery = useQuery({ queryKey: ['document', topicId], queryFn: () => api.getDocument(topicId) })
  const [draft, setDraft] = useState<string | null>(null)
  const [mode, setMode] = useState<DocumentMode>('edit')
  const [uploading, setUploading] = useState(false)
  const [uploadError, setUploadError] = useState<string>()
  const [retryFile, setRetryFile] = useState<File>()
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const content = draft ?? documentQuery.data?.content ?? ''
  const savedContent = documentQuery.data?.content ?? ''
  const dirty = draft !== null && draft !== savedContent
  const save = useMutation({
    mutationFn: (next: string) => api.setDocument(topicId, next),
    onSuccess: (next) => {
      queryClient.setQueryData(['document', topicId], next)
      setDraft(null)
    },
  })

  useEffect(() => {
    const field = textareaRef.current
    if (!field) return
    field.style.height = 'auto'
    field.style.height = `${Math.max(150, field.scrollHeight)}px`
  }, [content, mode])

  useEffect(() => {
    if (!dirty) return
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', onBeforeUnload)
    return () => window.removeEventListener('beforeunload', onBeforeUnload)
  }, [dirty])

  const updateDraft = (next: string) => setDraft(next)
  const insert = (prefix: string, suffix = '', empty = 'текст') => {
    const field = textareaRef.current
    if (!field) return
    const start = field.selectionStart
    const end = field.selectionEnd
    const selected = content.slice(start, end) || empty
    const next = `${content.slice(0, start)}${prefix}${selected}${suffix}${content.slice(end)}`
    updateDraft(next)
    requestAnimationFrame(() => {
      field.focus()
      const cursor = start + prefix.length + selected.length + suffix.length
      field.setSelectionRange(cursor, cursor)
    })
  }
  const insertList = () => {
    const field = textareaRef.current
    if (!field) return
    const start = field.selectionStart
    const end = field.selectionEnd
    const selected = content.slice(start, end) || 'элемент'
    const next = `${content.slice(0, start)}- ${selected.replace(/\n/g, '\n- ')}${content.slice(end)}`
    updateDraft(next)
    requestAnimationFrame(() => field.focus())
  }
  const insertImage = async (file?: File) => {
    if (!file || !file.type.startsWith('image/')) return
    setUploading(true); setUploadError(undefined); setRetryFile(file)
    try { const uploaded = await api.uploadImage(topicId, file); if (!uploaded.id) throw new Error('Не удалось получить адрес изображения'); updateDraft(`${content}\n![${file.name}](/api/uploads/images/${encodeURIComponent(uploaded.id)})\n`); setRetryFile(undefined) } catch { setUploadError('Изображение не загрузилось. Попробуйте ещё раз.') } finally { setUploading(false) }
  }

  if (documentQuery.isPending) return <section className="topic-document" aria-label="Конспект"><div className="document-heading"><h3>Конспект</h3><span className="field-hint" role="status">Загрузка…</span></div></section>
  if (documentQuery.isError) return <section className="topic-document" aria-label="Конспект"><div className="document-heading"><h3>Конспект</h3></div><ErrorMessage error={documentQuery.error} /><button className="button secondary" type="button" onClick={() => void documentQuery.refetch()}>Повторить</button></section>

  return (
    <section className="topic-document" aria-labelledby="topic-document-title">
      <div className="document-heading">
        <div><p className="eyebrow">Markdown</p><h3 id="topic-document-title">Конспект</h3></div>
        <span className={`document-state ${dirty ? 'is-dirty' : ''}`} role="status">{save.isPending ? 'Сохраняем…' : dirty ? 'Есть изменения' : 'Сохранено'}</span>
      </div>
      <div className="document-tabs" role="tablist" aria-label="Режим конспекта">
        <button type="button" role="tab" aria-selected={mode === 'edit'} className={mode === 'edit' ? 'active' : ''} onClick={() => setMode('edit')}><Icon name="edit" /> Редактор</button>
        <button type="button" role="tab" aria-selected={mode === 'preview'} className={mode === 'preview' ? 'active' : ''} onClick={() => setMode('preview')}><Icon name="note" /> Просмотр</button>
      </div>
      {mode === 'edit' ? (
        <>
          <div className="document-toolbar" role="toolbar" aria-label="Форматирование Markdown" onDrop={(event) => { event.preventDefault(); insertImage(event.dataTransfer.files[0]) }} onDragOver={(event) => event.preventDefault()}>
            <button type="button" onClick={() => insert('## ', '', 'Заголовок')} aria-label="Добавить заголовок">H</button>
            <button type="button" onClick={() => insert('**', '**', 'жирный текст')} aria-label="Жирный текст"><strong>B</strong></button>
            <button type="button" onClick={insertList} aria-label="Маркированный список"><Icon name="list" /></button>
            <button type="button" onClick={() => insert('[', '](https://)', 'текст ссылки')} aria-label="Добавить ссылку"><Icon name="link" /></button>
            <button type="button" onClick={() => insert('`', '`', 'код')} aria-label="Встроенный код"><span className="code-glyph">&lt;/&gt;</span></button>
            <label className="document-image-button"><input type="file" accept="image/*" onChange={(event) => void insertImage(event.target.files?.[0])} /> {uploading ? 'Загрузка…' : 'Изображение'}</label>
          </div>
          <textarea ref={textareaRef} className="document-editor" value={content} onChange={(event) => updateDraft(event.target.value)} onPaste={(event) => { const image = [...event.clipboardData.files].find((file) => file.type.startsWith('image/')); if (image) { event.preventDefault(); insertImage(image) } }} placeholder="Добавьте структурированный конспект…" aria-label="Текст конспекта" />
        </>
      ) : (
        <div className="document-preview markdown-body" aria-live="polite">
          {content.trim() ? <ReactMarkdown remarkPlugins={[remarkGfm]} skipHtml>{content}</ReactMarkdown> : <p className="muted">Конспект пока пуст.</p>}
        </div>
      )}
      {uploadError && <p className="form-error" role="alert">{uploadError} {retryFile && <button type="button" className="text-button" onClick={() => void insertImage(retryFile)}>Повторить</button>}</p>}
      <div className="document-footer">
        <span className="field-hint">{wordCount(content)} слов{documentQuery.data?.updated_at ? ` · ${formatUpdated(documentQuery.data.updated_at)}` : ''}</span>
        <button type="button" className="button primary" disabled={!dirty || save.isPending} onClick={() => save.mutate(content)}>{save.isPending ? 'Сохраняем…' : 'Сохранить'}</button>
      </div>
      <ErrorMessage error={save.error} />
    </section>
  )
}

export function TopicDocumentDialog({ topicId, onClose }: { topicId: string; onClose: () => void }) {
  return (
    <Dialog title="Конспект темы" className="document-dialog" onClose={onClose}>
      <div className="dialog-body document-dialog-body">
        <TopicDocument topicId={topicId} />
      </div>
    </Dialog>
  )
}

function wordCount(value: string) {
  return value.trim() ? value.trim().split(/\s+/u).length : 0
}

function formatUpdated(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.valueOf()) ? 'обновлено' : `обновлено ${date.toLocaleDateString('ru-RU')}`
}
