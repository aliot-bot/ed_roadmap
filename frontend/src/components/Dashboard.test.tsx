import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { Dashboard } from './Dashboard'
import { api } from '../api'

vi.mock('../api', () => ({ api: {
  getLearningOverview: vi.fn().mockResolvedValue({ study_minutes: 0, topic_minutes: 0, task_minutes: 0, active_days: 0 }),
  getLearningActivity: vi.fn().mockResolvedValue([{ date: '2026-10-12', study_minutes: 120 }]),
  getLearningAchievements: vi.fn().mockResolvedValue([{ kind: '4h', days: 2 }, { kind: '6h', days: 1 }, { kind: '8h', days: 1 }]),
  getAIAnalysis: vi.fn().mockResolvedValue({}), runAIAnalysis: vi.fn(),
} }))

describe('Dashboard activity', () => {
  it('renders a 12-month accessible week grid and backend achievement counts', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(<QueryClientProvider client={client}><Dashboard /></QueryClientProvider>)
    expect(await screen.findByRole('grid', { name: 'Активность обучения за последние 12 месяцев' })).toBeInTheDocument()
    expect(screen.getAllByRole('gridcell')).toHaveLength(371)
    expect(await screen.findByText('4 часа: 2 дн.')).toBeInTheDocument()
    expect(api.getLearningActivity).toHaveBeenCalled()
    const previous = screen.getByRole('button', { name: 'Предыдущий период' })
    fireEvent.click(previous)
    await waitFor(() => expect(api.getLearningActivity).toHaveBeenCalledTimes(2))
    expect((api.getLearningActivity as ReturnType<typeof vi.fn>).mock.calls[1]?.[0]).not.toEqual((api.getLearningActivity as ReturnType<typeof vi.fn>).mock.calls[0]?.[0])
  })
})
