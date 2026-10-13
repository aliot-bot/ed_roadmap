import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { TopicDocument, TopicDocumentDialog } from './TopicDocument'

const { getDocument, setDocument } = vi.hoisted(() => ({
  getDocument: vi.fn(),
  setDocument: vi.fn(),
}))

vi.mock('../api', () => ({
  api: {
    getDocument,
    setDocument,
  },
}))

afterEach(() => {
  getDocument.mockReset()
  setDocument.mockReset()
})

function renderDocument(content: string) {
  getDocument.mockResolvedValue({ content, updated_at: '2026-10-11T12:00:00Z' })
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <TopicDocument topicId="topic-1" />
    </QueryClientProvider>,
  )
}

function renderDocumentDialog(content: string) {
  getDocument.mockResolvedValue({ content, updated_at: '2026-10-11T12:00:00Z' })
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  HTMLDialogElement.prototype.showModal = vi.fn()
  HTMLDialogElement.prototype.close = vi.fn()
  return render(
    <QueryClientProvider client={queryClient}>
      <TopicDocumentDialog topicId="topic-1" onClose={vi.fn()} />
    </QueryClientProvider>,
  )
}

describe('TopicDocument', () => {
  it('renders Markdown preview without interpreting raw HTML', async () => {
    renderDocument('# Safe title\n\n<img src="x" onerror="alert(1)">\n\n**bold**')

    await screen.findByDisplayValue(/Safe title/)
    fireEvent.click(screen.getByRole('tab', { name: 'Просмотр' }))

    expect(await screen.findByRole('heading', { name: 'Safe title' })).toBeInTheDocument()
    expect(screen.getByText('bold')).toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
    expect(document.querySelector('[onerror]')).toBeNull()
  })

  it('saves edited Markdown through the document API', async () => {
    setDocument.mockResolvedValue({ content: '# Updated', updated_at: '2026-10-11T13:00:00Z' })
    renderDocument('# Initial')

    const editor = await screen.findByDisplayValue('# Initial')
    fireEvent.change(editor, { target: { value: '# Updated' } })
    fireEvent.click(screen.getByRole('button', { name: 'Сохранить' }))

    await waitFor(() => expect(setDocument).toHaveBeenCalledWith('topic-1', '# Updated'))
    expect(await screen.findByText('Сохранено')).toBeInTheDocument()
  })

  it('keeps the document inside the shared dialog content inset', async () => {
    const { container } = renderDocumentDialog('# Initial')

    await screen.findByDisplayValue('# Initial')
    expect(container.querySelector('.document-dialog-body > .topic-document')).toBeInTheDocument()
  })
})
