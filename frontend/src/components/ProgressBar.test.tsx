import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ProgressBar } from './ProgressBar'

describe('ProgressBar', () => {
  it('shows a meaningful empty state when there are no leaf topics', () => {
    render(<ProgressBar progress={{ completed: 0, total: 0, percent: null }} />)
    expect(screen.getByText('Нет тем')).toBeInTheDocument()
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
  })

  it('exposes completed count and percentage to people using assistive technology', () => {
    render(<ProgressBar progress={{ completed: 3, total: 4, percent: 75 }} />)
    expect(screen.getByText('3 из 4')).toBeInTheDocument()
    expect(screen.getByText('75%')).toBeInTheDocument()
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '75')
  })
})
