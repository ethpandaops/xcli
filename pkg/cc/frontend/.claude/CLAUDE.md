# CC Frontend

## Commands

```bash
pnpm dev                    # Dev server proxying to backend on :19280
pnpm lint                   # ESLint with custom color rules
pnpm test                   # Run all tests (unit)
pnpm test:unit              # Vitest unit tests only
pnpm build                  # Production build
pnpm typecheck              # Type check without emitting
pnpm format                 # Prettier format
pnpm storybook              # Storybook dev server on :6006
pnpm build-storybook        # Build static Storybook
```

## Libraries

- pnpm v10, vite v6, react v19, typescript v5
- tailwindcss v4
- storybook v10, vitest v4, msw v2

## Project Structure

```bash
src/
  App.tsx                     # Root app with page routing
  main.tsx                    # Entry point
  index.css                   # Global styles, theme tokens, animations

  components/                 # All UI components (flat structure)
    Dashboard.tsx             # Main dashboard with services, logs, stack controls
    ConfigPage.tsx            # Config management with tabs
    ConfigPanel.tsx           # Right sidebar config summary
    Header.tsx                # Top header bar with stack controls
    ServiceCard.tsx           # Service card with actions
    GitStatus.tsx             # Git repo status panel
    LogViewer.tsx             # Log viewer with filters
    StackProgress.tsx         # Boot/stop progress timeline
    LabConfigEditor.tsx       # Lab config form editor
    ServiceConfigViewer.tsx   # Service config file viewer/editor
    CBTOverridesEditor.tsx    # CBT model overrides manager
    InfraPanel.tsx            # Infrastructure status panel
    Spinner.tsx               # Loading spinner component

  hooks/                      # Custom React hooks
    useAPI.ts                 # API fetch/post/put/delete helpers
    useSSE.ts                 # Server-sent events connection
    useFavicon.ts             # Dynamic favicon based on stack status
    useNotifications.ts       # Browser notification API wrapper

  types/                      # TypeScript types
    index.ts                  # All shared type definitions
```

## Architecture Patterns

### Imports

Use `@/` path alias for all imports (not relative):

```tsx
import { useAPI } from '@/hooks/useAPI';
import type { ServiceInfo } from '@/types';
import Header from '@/components/Header';
```

### Color System

Custom color tokens defined in `src/index.css` via `@theme`:
- `surface`, `surface-light`, `surface-lighter` — background layers
- `border` — border color
- `bg` — page background

Custom ESLint rules (`cc/no-hardcoded-colors`, `cc/no-primitive-color-scales`) warn on non-token usage.

### State Management

- Local React state (useState/useReducer) — no external state library
- SSE (Server-Sent Events) for real-time updates via `useSSE` hook
- `useAPI` hook for REST API calls

## Development Guidelines

- Use semantic color tokens — avoid hardcoded hex/rgb colors
- Use `@/` path alias for imports
- Create Storybook stories for new components
- Write Vitest tests for hooks and utilities
- Run `pnpm lint` and `pnpm build` before committing
- Use Tailwind v4 class naming (`bg-linear-to-*`, `shadow-xs`, etc.)

### Naming Conventions

- **Components** (`.tsx`): PascalCase — `Dashboard.tsx`, `ServiceCard.tsx`
- **Hooks** (`.ts`): camelCase starting with `use` — `useAPI.ts`
- **Types** (`.ts`): kebab-case — `index.ts`

## Additional Rules

Detailed standards in `.claude/rules/`:

- [Loading States](.claude/rules/loading-states.md) — Skeleton/spinner patterns
- [Storybook](.claude/rules/storybook.md) — Story conventions
- [Theming](.claude/rules/theming.md) — Color token system
