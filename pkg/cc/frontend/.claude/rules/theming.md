# Theming

Color tokens defined in `src/index.css` via `@theme`:

## Tokens

- Surface: `surface`, `surface-light`, `surface-lighter`
- Border: `border`
- Background: `bg`
- Font: `mono` (JetBrains Mono)

## Usage

Always use token classes — never hardcoded colors:

```tsx
className="bg-surface text-white border-border"
className="bg-surface-light"
className="bg-surface-lighter"
```

Programmatic access:

```tsx
style={{ backgroundColor: 'var(--color-surface)' }}
style={{ borderColor: 'var(--color-border)' }}
```

## Modifying theme

Edit `@theme` block in `src/index.css`.

## ESLint enforcement

Custom rules warn on regressions:
- `cc/no-hardcoded-colors` — warns on hex/rgb/hsl in className and style props
- `cc/no-primitive-color-scales` — warns on `neutral-*` in className
