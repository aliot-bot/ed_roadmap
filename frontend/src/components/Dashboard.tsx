import { useMemo, useState } from 'react'
import {
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import ReactMarkdown from 'react-markdown'

import { api } from '../api'
import { Icon } from './Icon'

// ============================================================
// TYPES
// ============================================================

type CalendarDay = {
  key: string
  minutes: number
  inRange: boolean
  isFuture: boolean
}

type CalendarMonth = {
  key: string
  label: string
  week: number
}

type CalendarRange = {
  start: Date
  fromDate: Date
  toDate: Date
  from: string
  to: string
  timezone: string
  weeks: number
}

type AchievementKind = '4h' | '6h' | '8h'

// ============================================================
// CONSTANTS
// ============================================================

const MS_PER_DAY = 24 * 60 * 60 * 1000

const WEEKDAYS = [
  'Вс',
  'Пн',
  'Вт',
  'Ср',
  'Чт',
  'Пт',
  'Сб',
]

const LEGEND = [
  ['0 мин', 0],
  ['1–29', 1],
  ['30–59', 2],
  ['60–119', 3],
  ['120+', 4],
] as const

// ============================================================
// DATE HELPERS
// ============================================================

/**
 * Возвращает локальную календарную дату в формате YYYY-MM-DD.
 *
 * Не используем toISOString(), поскольку он преобразует
 * дату в UTC и может изменить календарный день.
 */
function formatLocalDate(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')

  return `${year}-${month}-${day}`
}

/**
 * Создаёт локальную дату без временной составляющей.
 */
function startOfDay(date: Date): Date {
  return new Date(
    date.getFullYear(),
    date.getMonth(),
    date.getDate(),
  )
}

/**
 * Добавляет указанное количество календарных дней.
 *
 * Используем setDate(), а не прибавление миллисекунд,
 * чтобы корректно работать при переходах DST.
 */
function addDays(date: Date, amount: number): Date {
  const result = new Date(date)

  result.setDate(result.getDate() + amount)

  return result
}

/**
 * Возвращает начало недели (воскресенье).
 */
function startOfWeek(date: Date): Date {
  const result = startOfDay(date)

  result.setDate(result.getDate() - result.getDay())

  return result
}

/**
 * Возвращает порядковый номер календарного дня через UTC.
 *
 * Это позволяет вычислять разницу между календарными
 * датами без ошибок из-за смены часового пояса или DST.
 */
function calendarDayNumber(date: Date): number {
  return Math.floor(
    Date.UTC(
      date.getFullYear(),
      date.getMonth(),
      date.getDate(),
    ) / MS_PER_DAY,
  )
}

/**
 * Разница между двумя календарными датами в днях.
 */
function differenceInCalendarDays(
  later: Date,
  earlier: Date,
): number {
  return (
    calendarDayNumber(later) -
    calendarDayNumber(earlier)
  )
}

/**
 * Уровень интенсивности активности.
 */
function getActivityLevel(minutes: number): number {
  if (minutes <= 0) return 0
  if (minutes < 30) return 1
  if (minutes < 60) return 2
  if (minutes < 120) return 3

  return 4
}

/**
 * Получает текущий часовой пояс пользователя.
 */
function getTimezone(): string {
  return (
    Intl.DateTimeFormat().resolvedOptions().timeZone ||
    'UTC'
  )
}

/**
 * Создаёт диапазон календаря на 12 месяцев.
 *
 * monthOffset:
 *   0  — текущий период
 *  -1  — период со сдвигом на месяц назад
 *  -2  — период со сдвигом на два месяца назад
 *
 * from включительно.
 * to исключительно.
 */
function createCalendarRange(
  monthOffset: number,
  today: Date,
): CalendarRange {
  // Начало месяца, следующего за конечным месяцем.
  const toDate = new Date(
    today.getFullYear(),
    today.getMonth() + monthOffset + 1,
    1,
  )

  // Начало первого месяца из выбранных 12.
  const fromDate = new Date(
    toDate.getFullYear(),
    toDate.getMonth() - 12,
    1,
  )

  // Выравниваем календарную сетку по воскресенью.
  const start = startOfWeek(fromDate)

  // Выравниваем конец сетки до целой недели.
  const lastDay = addDays(toDate, -1)

  const lastWeekStart = startOfWeek(lastDay)

  const gridEnd = addDays(lastWeekStart, 7)

  const totalDays = differenceInCalendarDays(
    gridEnd,
    start,
  )

  const weeks = totalDays / 7

  return {
    start,
    fromDate,
    toDate,
    from: formatLocalDate(fromDate),
    to: formatLocalDate(toDate),
    timezone: getTimezone(),
    weeks,
  }
}

// Представление одной части календаря (год, полугодие или квартал).
type CalendarSegment = {
  days: CalendarDay[]
  months: CalendarMonth[]
  weeks: number
}

function splitCalendar(
  days: CalendarDay[],
  months: CalendarMonth[],
  totalWeeks: number,
  monthsPerPart: number,
): CalendarSegment[] {
  const segments: CalendarSegment[] = []

  for (let index = 0; index < 12; index += monthsPerPart) {
    const firstMonth = months[index]
    if (!firstMonth) break

    const startWeek = firstMonth.week
    const endWeek = months[index + monthsPerPart]?.week ?? totalWeeks

    segments.push({
      days: days.slice(startWeek * 7, endWeek * 7),
      months: months.slice(index, index + monthsPerPart).map((month) => ({
        ...month,
        week: month.week - startWeek,
      })),
      weeks: endWeek - startWeek,
    })
  }

  return segments
}

/** Сетка всегда помещается в ширину родителя, без горизонтального скролла. */
function ActivityHeatmap({ days, months, weeks, accessible = false }: CalendarSegment & { accessible?: boolean }) {
  return (
    <div className="activity-scroll">
      <div
        className="activity-calendar-track"
        style={{
          width: `min(100%, calc(${weeks} * (var(--activity-max-cell) + var(--activity-gap)) - var(--activity-gap) + 28px))`,
        }}
      >
        <div className="activity-month-labels" aria-hidden="true">
          {months.map((month) => (
            <span
              key={month.key}
              style={{
                left: `calc(${((month.week / weeks) * 100).toFixed(5)}% + ${(month.week / weeks).toFixed(5)} * var(--activity-gap))`,
              }}
            >
              {month.label}
            </span>
          ))}
        </div>

        <div className="activity-calendar-body">
          <div className="activity-weekdays" aria-hidden="true">
            {WEEKDAYS.map((day) => (
              <span key={day}>{day}</span>
            ))}
          </div>

          <div
            className="activity-grid"
            role={accessible ? 'grid' : undefined}
            aria-label={accessible ? 'Активность обучения за последние 12 месяцев' : undefined}
            aria-hidden={accessible ? undefined : true}
            style={{
              gridTemplateColumns: `repeat(${weeks}, minmax(0, 1fr))`,
            }}
          >
            {days.map((day) => {
              const level = day.inRange ? getActivityLevel(day.minutes) : 0
              const description = day.inRange
                ? `${day.key}: ${day.minutes} минут`
                : day.isFuture
                  ? `${day.key}: будущая дата`
                  : `${day.key}: вне выбранного периода`

              return (
                <span
                  key={day.key}
                  className={[
                    'activity-cell',
                    `level-${level}`,
                    !day.inRange ? 'activity-cell-outside' : '',
                  ].filter(Boolean).join(' ')}
                  title={description}
                  role={accessible ? 'gridcell' : undefined}
                  aria-label={accessible ? description : undefined}
                />
              )
            })}
          </div>
        </div>
      </div>
    </div>
  )
}

// ============================================================
// DASHBOARD
// ============================================================

export function Dashboard({
  login,
}: {
  login?: string
}) {
  const queryClient = useQueryClient()

  // ----------------------------------------------------------
  // STATE
  // ----------------------------------------------------------

  const [monthOffset, setMonthOffset] = useState(0)

  // Текущая дата на момент рендера.
  const today = useMemo(() => startOfDay(new Date()), [])

  const todayKey = formatLocalDate(today)

  // ----------------------------------------------------------
  // CALENDAR RANGE
  // ----------------------------------------------------------

  const calendar = useMemo(
    () => createCalendarRange(monthOffset, today),
    [monthOffset, today],
  )

  const {
    start,
    fromDate,
    from,
    to,
    timezone,
    weeks,
  } = calendar

  // ----------------------------------------------------------
  // QUERIES
  // ----------------------------------------------------------

  const overview = useQuery({
    queryKey: ['learning-overview'],
    queryFn: () => api.getLearningOverview(),
  })

  const activity = useQuery({
    queryKey: [
      'learning-activity',
      from,
      to,
      timezone,
    ],

    queryFn: () =>
      api.getLearningActivity({
        from,
        to,
        timezone,
      }),
  })

  const achievements = useQuery({
    queryKey: ['learning-achievements'],
    queryFn: () => api.getLearningAchievements(),
  })

  const ai = useQuery({
    queryKey: ['ai-analysis'],
    queryFn: () => api.getAIAnalysis(),
    retry: false,
  })

  // ----------------------------------------------------------
  // AI ANALYSIS MUTATION
  // ----------------------------------------------------------

  const run = useMutation({
    mutationFn: () => api.runAIAnalysis(),

    onSuccess: (data) => {
      queryClient.setQueryData(
        ['ai-analysis'],
        data,
      )
    },
  })

  // ----------------------------------------------------------
  // ACTIVITY DATA
  // ----------------------------------------------------------

  /**
   * Создаём Map:
   *
   * YYYY-MM-DD -> study_minutes
   *
   * Предполагаем, что API возвращает даты уже
   * агрегированные по выбранному часовому поясу.
   *
   * Если несколько записей имеют одинаковую дату,
   * суммируем их значения.
   */
  const byDate = useMemo(() => {
    const result = new Map<string, number>()

    for (const item of activity.data ?? []) {
      const key = item.date.slice(0, 10)

      if (!/^\d{4}-\d{2}-\d{2}$/.test(key)) {
        continue
      }

      const minutes = Number(item.study_minutes)

      if (!Number.isFinite(minutes)) {
        continue
      }

      result.set(
        key,
        (result.get(key) ?? 0) + Math.max(0, minutes),
      )
    }

    return result
  }, [activity.data])

  // ----------------------------------------------------------
  // CALENDAR DAYS
  // ----------------------------------------------------------

  const days = useMemo<CalendarDay[]>(() => {
    const totalDays = weeks * 7

    return Array.from(
      { length: totalDays },
      (_, index) => {
        const date = addDays(start, index)

        const key = formatLocalDate(date)

        const isFuture = key > todayKey

        const inPeriod =
          key >= from &&
          key < to

        const inRange =
          inPeriod && !isFuture

        return {
          key,

          minutes: inRange
            ? (byDate.get(key) ?? 0)
            : 0,

          inRange,

          isFuture,
        }
      },
    )
  }, [
    start,
    weeks,
    from,
    to,
    todayKey,
    byDate,
  ])

  // ----------------------------------------------------------
  // MONTH LABELS
  // ----------------------------------------------------------

  const months = useMemo<CalendarMonth[]>(() => {
    return Array.from(
      { length: 12 },
      (_, index) => {
        const date = new Date(
          fromDate.getFullYear(),
          fromDate.getMonth() + index,
          1,
        )

        const daysFromStart =
          differenceInCalendarDays(
            date,
            start,
          )

        const week = Math.floor(
          daysFromStart / 7,
        )

        return {
          key: formatLocalDate(date),

          label: date.toLocaleDateString(
            'ru-RU',
            {
              month: 'short',
            },
          ),

          week,
        }
      },
    )
  }, [fromDate, start])

  // На планшете — два полугодия, на телефоне — четыре квартала.
  // Так весь год виден без горизонтальной прокрутки, а дни не становятся точками.
  const halves = useMemo(
    () => splitCalendar(days, months, weeks, 6),
    [days, months, weeks],
  )
  const quarters = useMemo(
    () => splitCalendar(days, months, weeks, 3),
    [days, months, weeks],
  )

  // ----------------------------------------------------------
  // ACHIEVEMENTS
  // ----------------------------------------------------------

  const getAchievement = (
    kind: AchievementKind,
  ): string => {
    if (achievements.isPending) {
      return '…'
    }

    if (achievements.isError) {
      return '—'
    }

    const value = achievements.data?.find(
      (item) => item.kind === kind,
    )

    return value
      ? String(value.days)
      : '—'
  }

  // ----------------------------------------------------------
  // NAVIGATION
  // ----------------------------------------------------------

  const handlePreviousPeriod = () => {
    setMonthOffset((current) => current - 1)
  }

  const handleNextPeriod = () => {
    setMonthOffset((current) =>
      Math.min(0, current + 1),
    )
  }

  const handleCurrentPeriod = () => {
    setMonthOffset(0)
  }

  // ----------------------------------------------------------
  // ACTIVITY STATUS
  // ----------------------------------------------------------

  const activityStatus = activity.isPending
    ? 'Загрузка…'
    : activity.isError
      ? 'Не удалось загрузить данные'
      : activity.data?.length
        ? 'Фактические минуты занятий'
        : 'Нет данных'

  // ----------------------------------------------------------
  // RENDER
  // ----------------------------------------------------------

  return (
    <section
      className="dashboard"
      aria-labelledby="dashboard-title"
    >
      {/* ====================================================
          HEADER
      ==================================================== */}

      <div className="dashboard-heading">
        <div>
          <p className="eyebrow">
            Обзор обучения
          </p>

          <h2 id="dashboard-title">
            Добрый день
            {login ? `, ${login}` : ''}
          </h2>

          <p className="dashboard-subtitle">
            Ваша учебная динамика и следующий фокус.
          </p>
        </div>

        <span
          className="dashboard-icon"
          aria-hidden="true"
        >
          <Icon name="sparkles" />
        </span>
      </div>

      {/* ====================================================
          METRICS
      ==================================================== */}

      <div className="dashboard-metrics">
        <Metric
          label="Минут учёбы"
          value={overview.data?.study_minutes}
          isLoading={overview.isPending}
          isError={overview.isError}
        />

        <Metric
          label="Минут по темам"
          value={overview.data?.topic_minutes}
          isLoading={overview.isPending}
          isError={overview.isError}
        />

        <Metric
          label="Активных дней"
          value={overview.data?.active_days}
          isLoading={overview.isPending}
          isError={overview.isError}
        />
      </div>

      {overview.isError && (
        <p className="muted" role="alert">
          Не удалось загрузить общую статистику.
        </p>
      )}

      {/* ====================================================
          ACTIVITY
      ==================================================== */}

      <div className="activity-wrap">
        {/* HEADER */}

        <div className="activity-heading">
          <div>
            <p className="eyebrow">
              Ритм
            </p>

            <h3>
              Активность за 12 месяцев
            </h3>
          </div>

          <span
            role={
              activity.isPending
                ? 'status'
                : undefined
            }
          >
            {activityStatus}
          </span>
        </div>

        {/* NAVIGATION */}

        <div className="activity-nav">
          <button
            type="button"
            className="icon-button"
            onClick={handlePreviousPeriod}
            aria-label="Предыдущий период"
          >
            ←
          </button>

          <button
            type="button"
            className="button tertiary compact-button"
            onClick={handleCurrentPeriod}
            disabled={monthOffset === 0}
          >
            Текущий период
          </button>

          <button
            type="button"
            className="icon-button"
            onClick={handleNextPeriod}
            disabled={monthOffset >= 0}
            aria-label="Следующий период"
          >
            →
          </button>
        </div>

        {/* CALENDAR */}

        <div className="activity-calendar-full">
          <ActivityHeatmap days={days} months={months} weeks={weeks} accessible />
        </div>

        <div className="activity-calendar-halves">
          {halves.map((segment, index) => (
            <ActivityHeatmap
              key={segment.months[0]?.key ?? `half-${index}`}
              {...segment}
            />
          ))}
        </div>

        <div className="activity-calendar-quarters">
          {quarters.map((segment, index) => (
            <ActivityHeatmap
              key={segment.months[0]?.key ?? `quarter-${index}`}
              {...segment}
            />
          ))}
        </div>

        {/* ACTIVITY ERROR */}

        {activity.isError && (
          <p className="muted" role="alert">
            Не удалось загрузить активность.

            <button
              type="button"
              className="button tertiary compact-button"
              onClick={() => {
                void activity.refetch()
              }}
            >
              Повторить
            </button>
          </p>
        )}

        {/* FOOTER */}

        <div className="activity-footer">
          {/* LEGEND */}

          <div
            className="activity-legend"
            aria-label="Легенда интенсивности"
          >
            {LEGEND.map(([label, value]) => (
              <span key={label}>
                <i
                  className={`activity-cell level-${value}`}
                  aria-hidden="true"
                />

                {label}
              </span>
            ))}
          </div>

          {/* ACHIEVEMENTS */}

          <div
            className="achievement-row"
            aria-label="Достижения за длительный день"
          >
            <Achievement
              label="4 часа"
              value={getAchievement('4h')}
            />

            <Achievement
              label="6 часов"
              value={getAchievement('6h')}
            />

            <Achievement
              label="8 часов"
              value={getAchievement('8h')}
            />
          </div>
        </div>
      </div>

      {/* ====================================================
          AI ANALYSIS
      ==================================================== */}

      <section
        className="ai-analysis"
        aria-labelledby="ai-analysis-title"
      >
        <div className="activity-heading">
          <div>
            <p className="eyebrow">
              Рекомендации
            </p>

            <h3 id="ai-analysis-title">
              Анализ прогресса
            </h3>
          </div>

          <button
            type="button"
            className="button tertiary compact-button"
            onClick={() => {
              run.mutate()
            }}
            disabled={run.isPending}
          >
            {run.isPending
              ? 'Готовим…'
              : 'Обновить анализ'}
          </button>
        </div>

        {/* GENERATION STATUS */}

        {run.isPending && (
          <p
            className="muted"
            role="status"
          >
            Формируем рекомендации на основе
            вашей учебной активности…
          </p>
        )}

        {/* GENERATION ERROR */}

        {run.isError && (
          <p
            className="muted"
            role="alert"
          >
            Не удалось обновить анализ.
            Попробуйте ещё раз.
          </p>
        )}

        {/* AI CONTENT */}

        {ai.isPending ? (
          <p
            className="muted"
            role="status"
          >
            Анализ загружается…
          </p>
        ) : ai.isError ||
          ai.data?.status === 'failed' ? (
          <p className="muted">
            Провайдер анализа сейчас недоступен.
          </p>
        ) : ai.data?.markdown ? (
          <div className="markdown-body">
            <ReactMarkdown skipHtml>
              {ai.data.markdown}
            </ReactMarkdown>
          </div>
        ) : (
          <p className="muted">
            Анализ пока не запускался.
          </p>
        )}
      </section>
    </section>
  )
}

// ============================================================
// METRIC
// ============================================================

type MetricProps = {
  label: string
  value?: number | null
  isLoading?: boolean
  isError?: boolean
}

function Metric({
  label,
  value,
  isLoading = false,
  isError = false,
}: MetricProps) {
  let displayValue: string

  if (isLoading) {
    displayValue = '…'
  } else if (isError) {
    displayValue = '—'
  } else {
    displayValue = String(value ?? 0)
  }

  return (
    <div className="dashboard-metric">
      <strong>
        {displayValue}
      </strong>

      <span>
        {label}
      </span>
    </div>
  )
}

// ============================================================
// ACHIEVEMENT
// ============================================================

type AchievementProps = {
  label: string
  value: string
}

function Achievement({
  label,
  value,
}: AchievementProps) {
  return (
    <span className="achievement">
      <span className="sr-only">
        {label}: {value} дн.
      </span>

      <strong aria-hidden="true">
        {value}
      </strong>

      <span aria-hidden="true">
        {label}
      </span>

      <small aria-hidden="true">
        дней
      </small>
    </span>
  )
}
