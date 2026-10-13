import {
  ArrowLeft,
  Check,
  ChevronRight,
  LayoutGrid,
  Link2,
  ListTree,
  LogOut,
  Map,
  Maximize2,
  Minimize2,
  Moon,
  Network,
  NotebookText,
  Pencil,
  Plus,
  Search,
  Sun,
  Trash2,
  X,
  Pause, Play, Sparkles,
  type LucideIcon,
} from 'lucide-react'

export type IconName = 'map' | 'logout' | 'plus' | 'trash' | 'edit' | 'arrow-left' | 'fit' | 'fullscreen' | 'exit-fullscreen' | 'layout' | 'graph' | 'list' | 'close' | 'check' | 'chevron' | 'note' | 'link' | 'sun' | 'moon' | 'search' | 'grid' | 'pause' | 'play' | 'sparkles'

const icons: Record<IconName, LucideIcon> = {
  map: Map,
  logout: LogOut,
  plus: Plus,
  trash: Trash2,
  edit: Pencil,
  'arrow-left': ArrowLeft,
  fit: Maximize2,
  fullscreen: Maximize2,
  'exit-fullscreen': Minimize2,
  layout: LayoutGrid,
  graph: Network,
  list: ListTree,
  close: X,
  check: Check,
  chevron: ChevronRight,
  note: NotebookText,
  link: Link2,
  sun: Sun,
  moon: Moon,
  search: Search,
  grid: LayoutGrid,
  pause: Pause,
  play: Play,
  sparkles: Sparkles,
}

export function Icon({ name }: { name: IconName }) {
  const Component = icons[name]
  return <Component aria-hidden="true" strokeWidth={1.8} />
}
