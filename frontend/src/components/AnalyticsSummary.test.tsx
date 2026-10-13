import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { AnalyticsSummary } from './AnalyticsSummary'

describe('AnalyticsSummary', () => {
  it('shows aggregate leaf-topic analytics and each roadmap breakdown', () => {
    render(<AnalyticsSummary
      analytics={{
        total: 3,
        completed: 2,
        remaining: 1,
        percent: 67,
        roadmaps: [
          { roadmap_id: 'one', title: 'First', total: 2, completed: 1, remaining: 1, percent: 50 },
          { roadmap_id: 'two', title: 'Second', total: 1, completed: 1, remaining: 0, percent: 100 },
        ],
      }}
      isLoading={false}
      isError={false}
    />)

    expect(screen.getByRole('heading', { name: 'Прогресс' })).toBeInTheDocument()
    expect(screen.getByText('Всего тем').previousElementSibling).toHaveTextContent('3')
    expect(screen.getByText('Изучено').previousElementSibling).toHaveTextContent('2')
    expect(screen.getByText('Осталось').previousElementSibling).toHaveTextContent('1')
    expect(screen.getByText('First')).toBeInTheDocument()
    expect(screen.getByText('Second')).toBeInTheDocument()
    expect(screen.getByRole('progressbar', { name: 'Общий прогресс 67%' })).toHaveAttribute('aria-valuenow', '67')
  })

  it('shows at most three roadmaps, prioritizing unfinished meaningful items', () => {
    render(<AnalyticsSummary
      analytics={{
        total: 8,
        completed: 4,
        remaining: 4,
        percent: 50,
        roadmaps: [
          { roadmap_id: 'empty', title: 'Empty', total: 0, completed: 0, remaining: 0, percent: 0 },
          { roadmap_id: 'done', title: 'Done', total: 2, completed: 2, remaining: 0, percent: 100 },
          { roadmap_id: 'active-one', title: 'Active one', total: 3, completed: 1, remaining: 2, percent: 33 },
          { roadmap_id: 'active-two', title: 'Active two', total: 3, completed: 2, remaining: 1, percent: 67 },
          { roadmap_id: 'extra', title: 'Extra', total: 4, completed: 1, remaining: 3, percent: 25 },
        ],
      }}
      isLoading={false}
      isError={false}
    />)

    expect(screen.getByText('Active one')).toBeInTheDocument()
    expect(screen.getByText('Active two')).toBeInTheDocument()
    expect(screen.getByText('Extra')).toBeInTheDocument()
    expect(screen.queryByText('Done')).not.toBeInTheDocument()
    expect(screen.queryByText('Empty')).not.toBeInTheDocument()
  })
})
