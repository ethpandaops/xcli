# Theming

Color tokens defined in `src/index.css` via `@theme`. CC is dark-only (no light mode).

## Surface Tokens

- `bg` — page background
- `surface` — sidebar/panel background
- `surface-light` — elevated cards
- `surface-lighter` — input backgrounds, subtle highlights
- `border` — borders, dividers

## Status Tokens

- `success` — running, healthy, enabled, "up to date" (emerald-400 equiv)
- `warning` — loading, pending, behind, degraded (amber-400 equiv)
- `error` — crashed, unhealthy, errors, danger actions (red-400 equiv)
- `info` — informational badges, healthy count, ahead (sky-400 equiv)

## Accent Tokens

- `accent` — buttons, focus rings, selected states, toggles, active tabs (indigo-500 equiv)
- `accent-light` — accent text on dark backgrounds, links (indigo-400 equiv)

## Text Hierarchy

- `text-primary` — white, headings, primary content
- `text-secondary` — names, labels, important secondary text (gray-300 equiv)
- `text-tertiary` — descriptions, action buttons, links (gray-400 equiv)
- `text-muted` — section labels, subtle text (gray-500 equiv)
- `text-disabled` — placeholders, very subtle elements (gray-600 equiv)

## Overlay

- `overlay` — modal/loading overlay backgrounds (black), use with opacity: `bg-overlay/60`

## Usage

Always use semantic token classes — never primitive Tailwind color scales:

```tsx
// Status
className="text-success"
className="bg-error/15 text-error ring-1 ring-error/25"
className="text-warning"

// Accent
className="bg-accent px-4 py-2 text-text-primary"
className="text-accent-light hover:text-accent-light/80"

// Text hierarchy
className="text-text-primary"
className="text-text-secondary"
className="text-text-muted"

// Surfaces
className="bg-surface border-border"

// Overlays
className="bg-overlay/60"
className="bg-overlay/80"
```

### Opacity Variants

Apply opacity via Tailwind `/N` syntax on any token:

```tsx
className="bg-success/20"      // success background at 20% opacity
className="ring-error/25"      // error ring at 25% opacity
className="text-warning/50"    // warning text at 50% opacity
```

## Programmatic Access

```tsx
style={{ backgroundColor: 'var(--color-surface)' }}
style={{ color: 'var(--color-success)' }}
style={{ boxShadow: '0 0 10px var(--color-warning)' }}
```

## Modifying Theme

Edit `@theme` block in `src/index.css`. All tokens use oklch values.

## ESLint Enforcement

Custom rules **error** on regressions:
- `cc/no-hardcoded-colors` — bans hex/rgb/hsl in className and style props
- `cc/no-primitive-color-scales` — bans all primitive Tailwind scales (gray, red, emerald, indigo, etc.)
