import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './api'

afterEach(() => vi.unstubAllGlobals())

describe('api.getNote', () => {
  it('reads note content from the dedicated topic endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ content: 'Важная заметка' }), {
      headers: { 'content-type': 'application/json' },
    }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.getNote('topic-1')).resolves.toBe('Важная заметка')
    expect(fetchMock).toHaveBeenCalledWith('/api/topics/topic-1/note', expect.objectContaining({ credentials: 'include' }))
  })
})

describe('learning and key contracts', () => {
  it('posts an access key without username fields', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ user: { id: 'u1', login: 'owner' } }), { headers: { 'content-type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(api.keyLogin('secret')).resolves.toEqual({ id: 'u1', login: 'owner' })
    expect(fetchMock).toHaveBeenCalledWith('/api/auth/key-login', expect.objectContaining({ body: JSON.stringify({ key: 'secret' }) }))
  })
  it('uploads images as multipart and returns stable id', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ id: 'img-1' }), { headers: { 'content-type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(api.uploadImage('topic-1', new File(['x'], 'a.png', { type: 'image/png' }))).resolves.toEqual({ id: 'img-1' })
    const body = (fetchMock.mock.calls[0]?.[1] as RequestInit).body as FormData
    expect(body).toBeInstanceOf(FormData)
    expect(body.get('topic_id')).toBe('topic-1')
  })
  it('uses learning analysis latest and run endpoints', async () => {
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(new Response(JSON.stringify({ content: '# Safe' }), { headers: { 'content-type': 'application/json' } })))
    vi.stubGlobal('fetch', fetchMock)
    await api.getAIAnalysis(); await api.runAIAnalysis()
    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/learning/analysis/latest')
    expect(fetchMock.mock.calls[1]?.[0]).toBe('/api/learning/analysis')
  })
})

describe('api.getAnalytics', () => {
  it('normalizes the backend leaf_topics contract for the dashboard', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      roadmap_count: 2,
      leaf_topics: { total: 3, completed: 2, remaining: 1, percent: 66.6666666667 },
      roadmaps: [
        { id: 'roadmap-1', title: 'First', leaf_topics: { total: 2, completed: 1, remaining: 1, percent: 50 } },
        { id: 'roadmap-2', title: 'Second', leaf_topics: { total: 1, completed: 1, remaining: 0, percent: 100 } },
      ],
    }), { headers: { 'content-type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)

    const result = await api.getAnalytics()
    expect(result).toMatchObject({ total: 3, completed: 2, remaining: 1, percent: 66.6666666667 })
    expect(result.roadmaps).toEqual([
      { roadmap_id: 'roadmap-1', title: 'First', total: 2, completed: 1, remaining: 1, percent: 50 },
      { roadmap_id: 'roadmap-2', title: 'Second', total: 1, completed: 1, remaining: 0, percent: 100 },
    ])
  })
})

describe('topic documents and analytics', () => {
  it('reads a Markdown document with its timestamp', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ content: '# Конспект', updated_at: '2026-10-11T12:00:00Z' }), {
      headers: { 'content-type': 'application/json' },
    }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.getDocument('topic-1')).resolves.toEqual({ content: '# Конспект', updated_at: '2026-10-11T12:00:00Z' })
    expect(fetchMock).toHaveBeenCalledWith('/api/topics/topic-1/document', expect.objectContaining({ credentials: 'include' }))
  })

  it('writes a Markdown document', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ content: '**Обновлено**' }), {
      headers: { 'content-type': 'application/json' },
    }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.setDocument('topic-1', '**Обновлено**')).resolves.toEqual({ content: '**Обновлено**' })
    expect(fetchMock).toHaveBeenCalledWith('/api/topics/topic-1/document', expect.objectContaining({ method: 'PUT', body: JSON.stringify({ content: '**Обновлено**' }) }))
  })

  it('uploads a roadmap cover as multipart data without forcing JSON content type', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ mime_type: 'image/png', updated_at: '2026-10-11T12:00:00Z' }), {
      headers: { 'content-type': 'application/json' },
    }))
    vi.stubGlobal('fetch', fetchMock)

    const file = new File(['png'], 'cover.png', { type: 'image/png' })
    await expect(api.uploadRoadmapCover('roadmap-1', file)).resolves.toMatchObject({ mime_type: 'image/png' })
    const options = fetchMock.mock.calls[0]?.[1] as RequestInit
    expect(options.method).toBe('PUT')
    expect(options.body).toBeInstanceOf(FormData)
    expect((options.headers as Record<string, string>)['Content-Type']).toBeUndefined()
  })

})
