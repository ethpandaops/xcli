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

  components/                 # Directory-per-component with barrel exports
    Dashboard/
      Dashboard.tsx           # Main dashboard with services, logs, stack controls
      index.ts
    ConfigPage/
      ConfigPage.tsx          # Config management with tabs
      index.ts
    ConfigPanel/
      ConfigPanel.tsx         # Right sidebar config summary
      index.ts
    Header/
      Header.tsx              # Top header bar with stack controls
      index.ts
    ServiceCard/
      ServiceCard.tsx         # Service card with actions
      index.ts
    GitStatus/
      GitStatus.tsx           # Git repo status panel
      index.ts
    LogViewer/
      LogViewer.tsx           # Log viewer with filters
      index.ts
    StackProgress/
      StackProgress.tsx       # Boot/stop progress timeline
      index.ts
    LabConfigEditor/
      LabConfigEditor.tsx     # Lab config form editor
      index.ts
    ServiceConfigViewer/
      ServiceConfigViewer.tsx # Service config file viewer/editor
      index.ts
    CBTOverridesEditor/
      CBTOverridesEditor.tsx  # CBT model overrides manager
      index.ts
    InfraPanel/
      InfraPanel.tsx          # Infrastructure status panel
      index.ts
    Spinner/
      Spinner.tsx             # Loading spinner component
      index.ts

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

Semantic color tokens defined in `src/index.css` via `@theme`:
- **Surface**: `bg`, `surface`, `surface-light`, `surface-lighter`, `border`
- **Status**: `success`, `warning`, `error`, `info`
- **Accent**: `accent`, `accent-light`
- **Text**: `text-primary`, `text-secondary`, `text-tertiary`, `text-muted`, `text-disabled`
- **Overlay**: `overlay` (use with opacity, e.g. `bg-overlay/60`)

Custom ESLint rules (`cc/no-hardcoded-colors`, `cc/no-primitive-color-scales`) **error** on non-token usage. See `.claude/rules/theming.md` for full details.

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
